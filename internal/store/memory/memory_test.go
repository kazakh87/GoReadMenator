package memory

import (
	"testing"

	"goreadmenator/internal/store"
	"goreadmenator/internal/store/storetest"
)

func TestContract(t *testing.T) {
	storetest.Run(t, func(t *testing.T) store.Store { return New() })
}
