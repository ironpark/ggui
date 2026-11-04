//go:build windows && amd64

#include "textflag.h"

// The Windows x64 convention puts the first four arguments in RCX, RDX, R8
// and R9 by position, and a floating-point argument in the XMM register of
// the same position instead. Go's callback entry saves only the integer
// four, so these thunks copy the floating-point ones across before jumping
// to it. Nothing is pushed, so the callback still sees the caller's return
// address and shadow space exactly where it expects them.

// ElementProviderFromPoint(this, double x, double y, ret):
// RCX holds this and R9 holds ret already; x and y are in X1 and X2.
TEXT ·winFromPointThunk(SB), NOSPLIT|NOFRAME, $0-0
	MOVQ	X1, DX
	MOVQ	X2, R8
	MOVQ	·winFromPointCB(SB), AX
	JMP	AX

// IRangeValueProvider::SetValue(this, double val): RCX holds this and the
// value is in X1.
TEXT ·winRangeSetThunk(SB), NOSPLIT|NOFRAME, $0-0
	MOVQ	X1, DX
	MOVQ	·winRangeSetCB(SB), AX
	JMP	AX

GLOBL ·winFromPointThunkAddr(SB), RODATA, $8
DATA ·winFromPointThunkAddr(SB)/8, $·winFromPointThunk(SB)

GLOBL ·winRangeSetThunkAddr(SB), RODATA, $8
DATA ·winRangeSetThunkAddr(SB)/8, $·winRangeSetThunk(SB)
