// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package conditionalaccess

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"reflect"
	"time"

	"github.com/hashicorp/go-azure-helpers/lang/pointer"
	"github.com/hashicorp/go-azure-sdk/sdk/odata"
	"github.com/hashicorp/go-uuid"
	"github.com/hashicorp/terraform-provider-azuread/internal/clients"
	"github.com/hashicorp/terraform-provider-azuread/internal/helpers"
	"github.com/hashicorp/terraform-provider-azuread/internal/tf"
	"github.com/hashicorp/terraform-provider-azuread/internal/tf/pluginsdk"
	"github.com/hashicorp/terraform-provider-azuread/internal/tf/validation"
	"github.com/manicminer/hamilton/msgraph"
)

func namedLocationResource() *pluginsdk.Resource {
	return &pluginsdk.Resource{
		CreateContext: namedLocationResourceCreate,
		ReadContext:   namedLocationResourceRead,
		UpdateContext: namedLocationResourceUpdate,
		DeleteContext: namedLocationResourceDelete,

		Timeouts: &pluginsdk.ResourceTimeout{
			Create: pluginsdk.DefaultTimeout(5 * time.Minute),
			Read:   pluginsdk.DefaultTimeout(5 * time.Minute),
			Update: pluginsdk.DefaultTimeout(5 * time.Minute),
			Delete: pluginsdk.DefaultTimeout(5 * time.Minute),
		},

		Importer: pluginsdk.ImporterValidatingResourceId(func(id string) error {
			if _, err := uuid.ParseUUID(id); err != nil {
				return fmt.Errorf("specified ID (%q) is not valid: %s", id, err)
			}
			return nil
		}),

		Schema: map[string]*pluginsdk.Schema{

			"display_name": {
				Type:             pluginsdk.TypeString,
				Required:         true,
				ValidateDiagFunc: validation.ValidateDiag(validation.StringIsNotEmpty),
			},

			"ip": {
				Type:         pluginsdk.TypeList,
				Optional:     true,
				ForceNew:     true,
				MaxItems:     1,
				ExactlyOneOf: []string{"ip", "country"},
				Elem: &pluginsdk.Resource{
					Schema: map[string]*pluginsdk.Schema{
						"ip_ranges": {
							Type:     pluginsdk.TypeList,
							Required: true,
							Elem: &pluginsdk.Schema{
								Type: pluginsdk.TypeString,
							},
						},

						"trusted": {
							Type:     pluginsdk.TypeBool,
							Optional: true,
						},
					},
				},
			},

			"country": {
				Type:         pluginsdk.TypeList,
				Optional:     true,
				ForceNew:     true,
				MaxItems:     1,
				ExactlyOneOf: []string{"ip", "country"},
				Elem: &pluginsdk.Resource{
					Schema: map[string]*pluginsdk.Schema{
						"countries_and_regions": {
							Type:     pluginsdk.TypeList,
							Required: true,
							Elem: &pluginsdk.Schema{
								Type: pluginsdk.TypeString,
							},
						},

						"include_unknown_countries_and_regions": {
							Type:     pluginsdk.TypeBool,
							Optional: true,
						},
					},
				},
			},
		},
	}
}

func namedLocationResourceCreate(ctx context.Context, d *pluginsdk.ResourceData, meta interface{}) pluginsdk.Diagnostics {
	client := meta.(*clients.Client).ConditionalAccess.NamedLocationsClient

	displayName := d.Get("display_name").(string)

	base := msgraph.BaseNamedLocation{
		DisplayName: pointer.To(displayName),
	}

	if v, ok := d.GetOk("ip"); ok {
		properties := expandIPNamedLocation(v.([]interface{}))
		properties.BaseNamedLocation = &base

		ipLocation, _, err := client.CreateIP(ctx, *properties)
		if err != nil {
			return tf.ErrorDiagF(err, "Could not create named location")
		}
		if ipLocation.ID == nil || *ipLocation.ID == "" {
			return tf.ErrorDiagF(errors.New("Bad API response"), "Object ID returned for named location is nil/empty")
		}

		d.SetId(*ipLocation.ID)
	} else if v, ok := d.GetOk("country"); ok {
		properties := expandCountryNamedLocation(v.([]interface{}))
		properties.BaseNamedLocation = &base

		countryLocation, _, err := client.CreateCountry(ctx, *properties)
		if err != nil {
			return tf.ErrorDiagF(err, "Could not create named location")
		}
		if countryLocation.ID == nil || *countryLocation.ID == "" {
			return tf.ErrorDiagF(errors.New("Bad API response"), "Object ID returned for named location is nil/empty")
		}

		d.SetId(*countryLocation.ID)
	} else {
		return tf.ErrorDiagF(errors.New("one of `ip` or `country` must be specified"), "Unable to determine named location type")
	}

	return namedLocationResourceRead(ctx, d, meta)
}

func namedLocationResourceUpdate(ctx context.Context, d *pluginsdk.ResourceData, meta interface{}) pluginsdk.Diagnostics {
	client := meta.(*clients.Client).ConditionalAccess.NamedLocationsClient

	base := msgraph.BaseNamedLocation{
		ID: pointer.To(d.Id()),
	}

	if d.HasChange("display_name") {
		displayName := d.Get("display_name").(string)
		base.DisplayName = &displayName
	}

	var updateRefreshFunc pluginsdk.StateRefreshFunc //nolint:staticcheck

	if v, ok := d.GetOk("ip"); ok {
		properties := expandIPNamedLocation(v.([]interface{}))
		properties.BaseNamedLocation = &base

		if _, err := client.UpdateIP(ctx, *properties); err != nil {
			return tf.ErrorDiagF(err, "Could not update named location with ID %q: %+v", d.Id(), err)
		}

		updateRefreshFunc = func() (interface{}, string, error) {
			result, _, err := client.GetIP(ctx, d.Id(), odata.Query{})
			if err != nil {
				return nil, "Error", err
			}

			if locationRaw := flattenIPNamedLocation(result); len(locationRaw) > 0 {
				location := locationRaw[0].(map[string]interface{})
				ip := v.([]interface{})[0].(map[string]interface{})
				if !reflect.DeepEqual(location["ip_ranges"], ip["ip_ranges"]) {
					return "stub", "Pending", nil
				}
				if location["trusted"].(bool) != ip["trusted"].(bool) {
					return "stub", "Pending", nil
				}
			}

			return "stub", "Updated", nil
		}
	}

	if v, ok := d.GetOk("country"); ok {
		properties := expandCountryNamedLocation(v.([]interface{}))
		properties.BaseNamedLocation = &base

		if _, err := client.UpdateCountry(ctx, *properties); err != nil {
			return tf.ErrorDiagF(err, "Could not update named location with ID %q: %+v", d.Id(), err)
		}

		updateRefreshFunc = func() (interface{}, string, error) {
			result, _, err := client.GetCountry(ctx, d.Id(), odata.Query{})
			if err != nil {
				return nil, "Error", err
			}

			if locationRaw := flattenCountryNamedLocation(result); len(locationRaw) > 0 {
				location := locationRaw[0].(map[string]interface{})
				ip := v.([]interface{})[0].(map[string]interface{})
				if !reflect.DeepEqual(location["countries_and_regions"], ip["countries_and_regions"]) {
					return "stub", "Pending", nil
				}
				if location["include_unknown_countries_and_regions"].(bool) != ip["include_unknown_countries_and_regions"].(bool) {
					return "stub", "Pending", nil
				}
			}

			return "stub", "Updated", nil
		}
	}

	log.Printf("[DEBUG] Waiting for named location %q to be updated", d.Id())
	timeout, _ := ctx.Deadline()
	stateConf := &pluginsdk.StateChangeConf{ //nolint:staticcheck
		Pending:                   []string{"Pending"},
		Target:                    []string{"Updated"},
		Timeout:                   time.Until(timeout),
		MinTimeout:                5 * time.Second,
		ContinuousTargetOccurence: 5,
		Refresh:                   updateRefreshFunc,
	}
	if _, err := stateConf.WaitForStateContext(ctx); err != nil {
		return tf.ErrorDiagF(err, "waiting for update of named location with ID %q", d.Id())
	}

	return namedLocationResourceRead(ctx, d, meta)
}

func namedLocationResourceRead(ctx context.Context, d *pluginsdk.ResourceData, meta interface{}) pluginsdk.Diagnostics {
	client := meta.(*clients.Client).ConditionalAccess.NamedLocationsClient

	result, status, err := client.Get(ctx, d.Id(), odata.Query{})
	if err != nil {
		if status == http.StatusNotFound {
			log.Printf("[DEBUG] Named Location with Object ID %q was not found - removing from state", d.Id())
			d.SetId("")
			return nil
		}
	}
	if result == nil {
		return tf.ErrorDiagF(errors.New("Bad API response"), "Result is nil")
	}

	location := *result

	if ipnl, ok := location.(msgraph.IPNamedLocation); ok {
		if ipnl.ID == nil {
			return tf.ErrorDiagF(errors.New("Bad API response"), "ID is nil for returned IP Named Location")
		}
		d.SetId(*ipnl.ID)
		tf.Set(d, "display_name", ipnl.DisplayName)
		tf.Set(d, "ip", flattenIPNamedLocation(&ipnl))
	}

	if cnl, ok := location.(msgraph.CountryNamedLocation); ok {
		if cnl.ID == nil {
			return tf.ErrorDiagF(errors.New("Bad API response"), "ID is nil for returned Country Named Location")
		}
		d.SetId(*cnl.ID)
		tf.Set(d, "display_name", cnl.DisplayName)
		tf.Set(d, "country", flattenCountryNamedLocation(&cnl))
	}

	return nil
}

func namedLocationResourceDelete(ctx context.Context, d *pluginsdk.ResourceData, meta interface{}) pluginsdk.Diagnostics {
	client := meta.(*clients.Client).ConditionalAccess.NamedLocationsClient
	namedLocationId := d.Id()

	if _, ok := d.GetOk("ip"); ok {
		resp, status, err := client.GetIP(ctx, namedLocationId, odata.Query{})
		if err != nil {
			if status == http.StatusNotFound {
				log.Printf("[DEBUG] Named Location with ID %q already deleted", namedLocationId)
				return nil
			}

			return tf.ErrorDiagPathF(err, "id", "Retrieving named location with ID %q", namedLocationId)
		}
		if resp != nil && resp.IsTrusted != nil && *resp.IsTrusted {
			properties := msgraph.IPNamedLocation{
				BaseNamedLocation: &msgraph.BaseNamedLocation{
					ID: &namedLocationId,
				},
				IsTrusted: pointer.To(false),
			}
			if _, err := client.UpdateIP(ctx, properties); err != nil {
				return tf.ErrorDiagF(err, "Updating named location with ID %q", namedLocationId)
			}
		}
	}

	if _, ok := d.GetOk("country"); ok {
		if _, status, err := client.GetCountry(ctx, namedLocationId, odata.Query{}); err != nil {
			if status == http.StatusNotFound {
				log.Printf("[DEBUG] Named Location with ID %q already deleted", namedLocationId)
				return nil
			}

			return tf.ErrorDiagPathF(err, "id", "Retrieving named location with ID %q", namedLocationId)
		}
	}

	status, err := client.Delete(ctx, namedLocationId)
	if err != nil {
		return tf.ErrorDiagPathF(err, "id", "Deleting named location with ID %q, got status %d", namedLocationId, status)
	}

	if err := helpers.WaitForDeletion(ctx, func(ctx context.Context) (*bool, error) {
		defer func() { client.BaseClient.DisableRetries = false }()
		client.BaseClient.DisableRetries = true
		if _, status, err := client.Get(ctx, namedLocationId, odata.Query{}); err != nil {
			if status == http.StatusNotFound {
				return pointer.To(false), nil
			}
			return nil, err
		}
		return pointer.To(true), nil
	}); err != nil {
		return tf.ErrorDiagF(err, "waiting for deletion of named location with ID %q", namedLocationId)
	}

	return nil
}
