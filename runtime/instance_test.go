//go:build !js

package runtime

import (
	"errors"
	"os"
	"slices"
	"testing"
	"time"
)

func TestClaimInstanceHandsLaterLaunchesToTheFirst(t *testing.T) {
	id := "ggui.test." + t.Name() + "." + time.Now().Format("150405.000000")
	got := make(chan Launch, 1)
	release, err := ClaimInstance(id, func(l Launch) { got <- l })
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := ClaimInstance(id, func(Launch) { t.Error("second instance was handed a launch") }); !errors.Is(err, ErrRunning) {
		t.Fatalf("second claim = %v, want ErrRunning", err)
	}
	select {
	case l := <-got:
		dir, _ := os.Getwd()
		if l.Dir != dir || !slices.Equal(l.Args, os.Args[1:]) {
			t.Fatalf("launch = %+v", l)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the first instance was not handed the launch")
	}
	release()
	// Released, the id is free again, even with the old socket left.
	again, err := ClaimInstance(id, func(Launch) {})
	if err != nil {
		t.Fatalf("claim after release = %v", err)
	}
	again()
}
