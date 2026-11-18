package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/senvu/senvu/internal/scanner"
)

func BenchmarkScanner1000Files(b *testing.B) {
	dir := b.TempDir()
	for i := 0; i < 1000; i++ {
		sub := filepath.Join(dir, fmt.Sprintf("sub_%d", i%10))
		os.MkdirAll(sub, 0755)
		os.WriteFile(filepath.Join(sub, fmt.Sprintf("file_%d.txt", i)), []byte("benchmark content payload"), 0644)
	}

	sc := scanner.NewScanner()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := sc.Scan(dir)
		if err != nil {
			b.Fatal(err)
		}
	}
}
