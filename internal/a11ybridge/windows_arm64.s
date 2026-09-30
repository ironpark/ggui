//go:build windows && arm64

#include "textflag.h"

// On ARM64 the integer and floating-point arguments are numbered
// separately, so a double does not consume an integer register: the
// arguments after it shift down rather than leaving a gap. The thunks
// therefore move the later integer arguments out of the way first.

// ElementProviderFromPoint(this, double x, double y, ret):
// R0 holds this, R1 holds ret, and x and y are in F0 and F1.
TEXT ·winFromPointThunk(SB), NOSPLIT|NOFRAME, $0-0
	MOVD	R1, R3
	FMOVD	F0, R1
	FMOVD	F1, R2
	MOVD	·winFromPointCB(SB), R12
	B	(R12)

// IRangeValueProvider::SetValue(this, double val): R0 holds this and the
// value is in F0.
TEXT ·winRangeSetThunk(SB), NOSPLIT|NOFRAME, $0-0
	FMOVD	F0, R1
	MOVD	·winRangeSetCB(SB), R12
	B	(R12)

GLOBL ·winFromPointThunkAddr(SB), RODATA, $8
DATA ·winFromPointThunkAddr(SB)/8, $·winFromPointThunk(SB)

GLOBL ·winRangeSetThunkAddr(SB), RODATA, $8
DATA ·winRangeSetThunkAddr(SB)/8, $·winRangeSetThunk(SB)
