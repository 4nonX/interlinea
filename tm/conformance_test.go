package tm_test

import (
	"testing"

	"github.com/4nonX/interlinea/tm"
	"github.com/4nonX/interlinea/tm/tmtest"
)

func TestInMemoryConformance(t *testing.T) {
	tmtest.Run(t, func(t *testing.T, info tm.Info) tm.Memory {
		m, err := tm.NewInMemory(info)
		if err != nil {
			t.Fatal(err)
		}
		return m
	})
}
