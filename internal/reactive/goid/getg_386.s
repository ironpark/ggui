#include "textflag.h"

// func getg() unsafe.Pointer
TEXT ·getg(SB), NOSPLIT, $0-4
	MOVL	(TLS), AX
	MOVL	AX, ret+0(FP)
	RET
