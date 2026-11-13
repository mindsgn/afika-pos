//go:build js && wasm

package posstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"syscall/js"

	"github.com/syumai/workers-go/cloudflare"
)

// D1BatchRunner executes write batches with D1Database.batch(), which
// Cloudflare runs as a single SQL transaction. It is the production write path
// for the Worker build; native builds and tests use database/sql transactions
// instead.
type D1BatchRunner struct {
	binding string
}

// NewD1BatchRunner binds the runner to a D1 binding (for example "afika").
func NewD1BatchRunner(binding string) *D1BatchRunner {
	return &D1BatchRunner{binding: binding}
}

// Run prepares and binds every statement, then submits the whole batch at once.
// If D1 reports an error for any statement the platform rolls the batch back,
// so a partially applied sale is impossible.
func (r *D1BatchRunner) Run(_ context.Context, statements []Statement) (err error) {
	if len(statements) == 0 {
		return nil
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("d1 batch panic: %v", recovered)
		}
	}()

	database := cloudflare.GetBinding(r.binding)
	if database.IsUndefined() || database.IsNull() {
		return fmt.Errorf("d1 binding %q is not configured", r.binding)
	}

	arrayClass := js.Global().Get("Array")
	batch := arrayClass.New(len(statements))
	for i, statement := range statements {
		prepared := database.Call("prepare", statement.SQL)
		if len(statement.Args) > 0 {
			args := make([]any, len(statement.Args))
			for j, arg := range statement.Args {
				args[j] = d1JSArgument(arg)
			}
			prepared = prepared.Call("bind", args...)
		}
		batch.SetIndex(i, prepared)
	}

	results, err := awaitJS(database.Call("batch", batch))
	if err != nil {
		return fmt.Errorf("d1 batch: %w", err)
	}
	if results.Length() != len(statements) {
		return fmt.Errorf("d1 batch returned %d results for %d statements", results.Length(), len(statements))
	}
	for i := 0; i < results.Length(); i++ {
		result := results.Index(i)
		success := result.Get("success")
		if success.Type() == js.TypeBoolean && !success.Bool() {
			return fmt.Errorf("d1 batch statement %d failed: %s", i, describeD1Error(result))
		}
	}
	return nil
}

// d1JSArgument converts database/sql argument shapes to values accepted by
// syscall/js. The SQL writer uses pointers for nullable numeric columns; a
// non-nil pointer must be dereferenced and a nil pointer must bind as SQL NULL.
func d1JSArgument(value any) any {
	if value == nil {
		return js.Null()
	}
	ref := reflect.ValueOf(value)
	for ref.Kind() == reflect.Pointer || ref.Kind() == reflect.Interface {
		if ref.IsNil() {
			return js.Null()
		}
		ref = ref.Elem()
	}
	return ref.Interface()
}

// describeD1Error renders the D1Result error payload as a readable string.
func describeD1Error(result js.Value) string {
	raw := result.Get("error")
	if raw.IsUndefined() || raw.IsNull() {
		return "unknown error"
	}
	var decoded any
	if err := json.Unmarshal([]byte(raw.String()), &decoded); err == nil {
		if text, ok := decoded.(string); ok {
			return text
		}
	}
	return raw.String()
}

// awaitJS waits for a JavaScript promise. Go's wasm scheduler runs the
// resolution callbacks on the event loop, so blocking on these channels is the
// supported way to await in a Worker.
func awaitJS(promise js.Value) (js.Value, error) {
	resultCh := make(chan js.Value, 1)
	errCh := make(chan error, 1)
	var then, catch js.Func
	then = js.FuncOf(func(_ js.Value, args []js.Value) any {
		defer then.Release()
		if len(args) == 0 {
			resultCh <- js.Undefined()
			return js.Undefined()
		}
		resultCh <- args[0]
		return js.Undefined()
	})
	catch = js.FuncOf(func(_ js.Value, args []js.Value) any {
		defer catch.Release()
		reason := js.Undefined()
		if len(args) > 0 {
			reason = args[0]
		}
		errCh <- errors.New(reason.Call("toString").String())
		return js.Undefined()
	})
	promise.Call("then", then).Call("catch", catch)

	select {
	case result := <-resultCh:
		return result, nil
	case err := <-errCh:
		return js.Undefined(), err
	}
}
