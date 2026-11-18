package imagevector

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestMappingUsesCLIPVectorDimensions(t *testing.T) {
	mapping := Mapping()
	raw, err := json.Marshal(mapping)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	properties := decoded["mappings"].(map[string]any)["properties"].(map[string]any)
	vector := properties["vector"].(map[string]any)
	if vector["type"] != "dense_vector" || vector["dims"] != float64(512) || vector["similarity"] != "cosine" {
		t.Fatalf("unexpected vector mapping: %#v", vector)
	}
}

func TestUniqueProductIDsPreservesSimilarityOrder(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	got := UniqueProductIDs([]uuid.UUID{a, a, uuid.Nil, b, b}, 2)
	if len(got) != 2 || got[0] != a || got[1] != b {
		t.Fatalf("got %v, want [%s %s]", got, a, b)
	}
}
