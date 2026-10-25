package tools

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tr4m0ryp/oido-google-calander/internal/account"
	"github.com/tr4m0ryp/oido-google-calander/internal/calendar"
)

// maxParallelAccounts bounds concurrent Google calls during "all" fan-out.
const maxParallelAccounts = 4

// readOnly marks tools that never modify calendars.
var readOnly = &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(true)}

func ptr[T any](v T) *T { return &v }

// jsonResult renders v as indented JSON text content.
func jsonResult(v any) (*mcp.CallToolResult, any, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(data)}}}, nil, nil
}

// perAccount is one account's share of a fan-out result. Failures are
// reported inline so the model can tell "no events" from "account broken".
type perAccount[T any] struct {
	Account string `json:"account"`
	Items   T      `json:"items"`
	Error   string `json:"error,omitempty"`
}

// fanOut runs fn for every account selected by arg, in parallel, keeping
// the provider's account order in the result.
func fanOut[T any](ctx context.Context, p account.Provider, arg string,
	fn func(context.Context, *calendar.Client) (T, error)) ([]perAccount[T], error) {
	emails, err := account.Resolve(p, arg, true)
	if err != nil {
		return nil, err
	}
	out := make([]perAccount[T], len(emails))
	sem := make(chan struct{}, maxParallelAccounts)
	var wg sync.WaitGroup
	for i, email := range emails {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			out[i].Account = email
			client, err := p.Client(ctx, email)
			if err == nil {
				out[i].Items, err = fn(ctx, client)
			}
			if err != nil {
				out[i].Error = err.Error()
			}
		}()
	}
	wg.Wait()
	return out, nil
}
