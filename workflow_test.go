package workflow

import (
	"context"
	"fmt"
	"log"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type testState string

func (s testState) String() string {
	return string(s)
}

type testTransit string

func (s testTransit) String() string {
	return string(s)
}

const (
	toNew    testTransit = "to new"
	toDone   testTransit = "to done"
	toCancel testTransit = "to cancel"
)
const (
	newState    testState = "new"
	doneState   testState = "done"
	cancelState testState = "cancel"
)

type testData struct {
	state fmt.Stringer
}

func (t testData) GetState() fmt.Stringer {
	return t.state
}

func ExampleNewWorkflow() {
	ctx := context.Background()
	w := NewWorkflow(func(ctx context.Context, data Data, dst fmt.Stringer) (Data, error) {
		// set state and return data with state
		// can persist to db
		d := data.(testData)
		d.state = dst
		return d, nil
	})
	// add transition
	if err := w.Add(toNew, &Transition{Dst: newState}); err != nil {
		log.Fatal(err)
	}
	// get your data with state
	d := testData{}

	// check can change state
	if !w.Can(d, toNew) {
		log.Fatal(d)
	}

	ex, err := w.Apply(ctx, d, toNew)
	if err != nil {
		log.Fatal(err)
	}
	// data with new state
	log.Print(ex)
}

func TestNewWorkflow(t *testing.T) {
	w := NewWorkflow(func(ctx context.Context, data Data, dst fmt.Stringer) (Data, error) {
		return nil, nil
	})
	require.NotNil(t, w.transitions)
	require.Len(t, w.transitions, 0)
	require.NotNil(t, w.apply)
}

func TestWorkflow_Apply(t *testing.T) {
	ctx := context.Background()
	w := NewWorkflow(func(ctx context.Context, data Data, dst fmt.Stringer) (Data, error) {
		d := data.(testData)
		d.state = dst
		return d, nil
	})
	require.Nil(t, w.Add(toNew, &Transition{Dst: newState}))
	require.Nil(t, w.Add(toDone, &Transition{Dst: doneState, Src: []fmt.Stringer{newState}}))
	data := testData{}
	ex, err := w.Apply(ctx, data, toDone)
	require.Nil(t, ex)
	require.EqualError(t, err, "transit not allowed")
	exNew, err := w.Apply(ctx, data, toNew)
	require.Nil(t, err)
	require.Equal(t, newState, exNew.GetState())

	exDone, err := w.Apply(ctx, exNew, toDone)
	require.Nil(t, err)
	require.Equal(t, doneState, exDone.GetState())
}

type testMWFactory struct {
	mu sync.Mutex
	ex []string
}

func (tr *testMWFactory) Success(t *testing.T, name string) Middleware {
	fn := func(ctx context.Context, data Data, next Process) (Data, error) {
		tr.mu.Lock()
		tr.ex = append(tr.ex, name)
		tr.mu.Unlock()
		return next(ctx, data)
	}
	return fn
}

func TestWorkflow_Apply_Middleware(t *testing.T) {
	ctx := context.Background()
	w := NewWorkflow(func(ctx context.Context, data Data, dst fmt.Stringer) (Data, error) {
		d := data.(testData)
		d.state = dst
		return d, nil
	})
	mwf := &testMWFactory{}
	require.Nil(t, w.Add(toNew, &Transition{Dst: newState}, mwf.Success(t, "new")))
	require.Nil(t, w.Add(toDone, &Transition{
		Dst:        doneState,
		Src:        []fmt.Stringer{newState},
		Middleware: mwf.Success(t, "done"),
	}))
	require.Nil(t, w.Add(toCancel,
		&Transition{
			Dst:        cancelState,
			Src:        []fmt.Stringer{newState, doneState},
			Middleware: mwf.Success(t, "cancel"),
		},
		mwf.Success(t, "cancel add 1"),
		mwf.Success(t, "cancel add 2"),
		mwf.Success(t, "cancel add 3"),
	))
	data := testData{}
	exNew, err := w.Apply(ctx, data, toNew)
	require.Nil(t, err)
	require.Equal(t, []string{"new"}, mwf.ex)

	exDone, err := w.Apply(ctx, exNew, toDone)
	require.Equal(t, []string{"new", "done"}, mwf.ex)
	require.Nil(t, err)

	exCancel, err := w.Apply(ctx, exDone, toCancel)
	require.Equal(t, []string{"new", "done", "cancel add 1", "cancel add 2", "cancel add 3", "cancel"}, mwf.ex)
	require.Nil(t, err)
	require.Equal(t, cancelState, exCancel.GetState())
}

func TestWorkflow_Add(t *testing.T) {
	w := NewWorkflow(func(ctx context.Context, data Data, dst fmt.Stringer) (Data, error) {
		return data, nil
	})

	require.Nil(t, w.Add(toNew, &Transition{Dst: newState}))
	require.EqualError(t, w.Add(toNew, &Transition{Dst: doneState}), "duplicate transit")
}

func TestWorkflow_Can(t *testing.T) {
	w := NewWorkflow(func(ctx context.Context, data Data, dst fmt.Stringer) (Data, error) {
		return data, nil
	})
	require.Nil(t, w.Add(toNew, &Transition{Dst: newState}))
	require.Nil(t, w.Add(toCancel, &Transition{Dst: cancelState, Src: []fmt.Stringer{newState}}))
	data := testData{}
	require.True(t, w.Can(data, toNew))
	require.False(t, w.Can(data, toCancel))
	require.False(t, w.Can(data, toDone))
}
