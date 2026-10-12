package mcpserver

import (
	"context"
	"testing"
)

// BenchmarkServerStartup measures New for each profile: tool registration with
// schema derivation, which every process start pays.
func BenchmarkServerStartup(b *testing.B) {
	root := newMCPBenchmarkWorkspace(b)
	for _, profile := range []Profile{ProfileAll, ProfileRuntime, ProfileCuration} {
		b.Run(string(profile), func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				if _, _, err := New(root, nil, profile); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkProxyRuntimeToolDefinitions measures how the proxy obtains the
// runtime tool list at every start.
func BenchmarkProxyRuntimeToolDefinitions(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		tools, err := runtimeToolDefinitions(context.Background())
		if err != nil {
			b.Fatal(err)
		}
		if len(tools) == 0 {
			b.Fatal("no runtime tools")
		}
	}
}
