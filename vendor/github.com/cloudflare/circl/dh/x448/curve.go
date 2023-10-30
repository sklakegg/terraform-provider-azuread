// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package x448

import (
	fp "github.com/cloudflare/circl/math/fp448"
)

// ladderJoye calculates a fixed-point multiplication with the generator point.
// The algorithm is the right-to-left Joye's ladder as described
// in "How to precompute a ladder" in SAC'2017.
func ladderJoye(k *Key) {
	w := [5]fp.Elt{} // [mu,x1,z1,x2,z2] order must be preserved.
	w[1] = fp.Elt{   // x1 = S
		0xfe, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xfe, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	}
	fp.SetOne(&w[2]) // z1 = 1
	w[3] = fp.Elt{   // x2 = G-S
		0x20, 0x27, 0x9d, 0xc9, 0x7d, 0x19, 0xb1, 0xac,
		0xf8, 0xba, 0x69, 0x1c, 0xff, 0x33, 0xac, 0x23,
		0x51, 0x1b, 0xce, 0x3a, 0x64, 0x65, 0xbd, 0xf1,
		0x23, 0xf8, 0xc1, 0x84, 0x9d, 0x45, 0x54, 0x29,
		0x67, 0xb9, 0x81, 0x1c, 0x03, 0xd1, 0xcd, 0xda,
		0x7b, 0xeb, 0xff, 0x1a, 0x88, 0x03, 0xcf, 0x3a,
		0x42, 0x44, 0x32, 0x01, 0x25, 0xb7, 0xfa, 0xf0,
	}
	fp.SetOne(&w[4]) // z2 = 1

	const n = 448
	const h = 2
	swap := uint(1)
	for s := 0; s < n-h; s++ {
		i := (s + h) / 8
		j := (s + h) % 8
		bit := uint((k[i] >> uint(j)) & 1)
		copy(w[0][:], tableGenerator[s*Size:(s+1)*Size])
		diffAdd(&w, swap^bit)
		swap = bit
	}
	for s := 0; s < h; s++ {
		double(&w[1], &w[2])
	}
	toAffine((*[fp.Size]byte)(k), &w[1], &w[2])
}

// ladderMontgomery calculates a generic scalar point multiplication
// The algorithm implemented is the left-to-right Montgomery's ladder.
func ladderMontgomery(k, xP *Key) {
	w := [5]fp.Elt{}      // [x1, x2, z2, x3, z3] order must be preserved.
	w[0] = *(*fp.Elt)(xP) // x1 = xP
	fp.SetOne(&w[1])      // x2 = 1
	w[3] = *(*fp.Elt)(xP) // x3 = xP
	fp.SetOne(&w[4])      // z3 = 1

	move := uint(0)
	for s := 448 - 1; s >= 0; s-- {
		i := s / 8
		j := s % 8
		bit := uint((k[i] >> uint(j)) & 1)
		ladderStep(&w, move^bit)
		move = bit
	}
	toAffine((*[fp.Size]byte)(k), &w[1], &w[2])
}

func toAffine(k *[fp.Size]byte, x, z *fp.Elt) {
	fp.Inv(z, z)
	fp.Mul(x, x, z)
	_ = fp.ToBytes(k[:], x)
}

var lowOrderPoints = [3]fp.Elt{
	{ /* (0,_,1) point of order 2 on Curve448 */
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	},
	{ /* (1,_,1) a point of order 4 on the twist of Curve448 */
		0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	},
	{ /* (-1,_,1) point of order 4 on Curve448 */
		0xfe, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xfe, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	},
}
