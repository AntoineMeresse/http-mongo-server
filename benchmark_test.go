package main

import (
	"encoding/json"
	"fmt"
	"testing"
)

func BenchmarkSprintfJSON(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = fmt.Sprintf(`{"name": "name%d", "key": "key%d"}`, i, i)
	}
}

func BenchmarkMarshalJSON(b *testing.B) {
	for i := 0; i < b.N; i++ {
		doc := MyDocument{Name: fmt.Sprintf("name%d", i), Key: fmt.Sprintf("key%d", i)}
		_, err := json.Marshal(doc)
		if err != nil {
			b.Fatal(err)
		}
	}
}
