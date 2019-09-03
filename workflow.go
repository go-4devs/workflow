package workflow

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// base errors
var (
	ErrTransitNotAllowed = errors.New("transit not allowed")
	ErrDuplicateTransit  = errors.New("duplicate transit")
)

// Data for the transit
type Data interface {
	GetState() fmt.Stringer
}

// Process set state for the data
type Process func(ctx context.Context, data Data) (Data, error)

// Middleware run other logic
type Middleware func(ctx context.Context, data Data, next Process) (Data, error)

// Transition configure
type Transition struct {
	Src        []fmt.Stringer
	Dst        fmt.Stringer
	Middleware Middleware
}

// Can check state by src
func (tr *Transition) Can(data Data) bool {
	if len(tr.Src) == 0 {
		return true
	}
	for _, src := range tr.Src {
		if data.GetState() == src {
			return true
		}
	}
	return false
}

// Apply state to data
type Apply func(ctx context.Context, data Data, dst fmt.Stringer) (Data, error)

// NewWorkflow create new workflow
func NewWorkflow(apply Apply, mw ...Middleware) *Workflow {
	return &Workflow{
		apply:       apply,
		mw:          chainProcess(mw...),
		transitions: make(map[fmt.Stringer]*Transition),
	}
}

// Workflow configure transitions
type Workflow struct {
	transitions map[fmt.Stringer]*Transition
	apply       Apply
	mw          Middleware
	mu          sync.Mutex
}

// Get transition by data and transit
func (w *Workflow) Get(data Data, transit fmt.Stringer) *Transition {
	tr, ok := w.transitions[transit]
	if !ok || !tr.Can(data) {
		return nil
	}
	return tr
}

// Add new transition and custom middleware
func (w *Workflow) Add(name fmt.Stringer, transit *Transition, mw ...Middleware) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.transitions[name]; ok {
		return ErrDuplicateTransit
	}

	if transit.Middleware != nil {
		mw = append(mw, transit.Middleware)
	}
	transit.Middleware = chainProcess(mw...)
	w.transitions[name] = transit

	return nil
}

// Can check can transit by src data
func (w *Workflow) Can(data Data, transit fmt.Stringer) bool {
	return w.Get(data, transit) != nil
}

// Apply transit with middleware
func (w *Workflow) Apply(ctx context.Context, data Data, transit fmt.Stringer) (Data, error) {
	return w.mw(ctx, data, func(ctx context.Context, data Data) (Data, error) {
		if tr := w.Get(data, transit); tr != nil {
			return tr.Middleware(ctx, data, func(ctx context.Context, data Data) (Data, error) {
				return w.apply(ctx, data, tr.Dst)
			})
		}
		return nil, ErrTransitNotAllowed
	})
}

// chainProcess add chain by Process
func chainProcess(handleFunc ...Middleware) Middleware {
	n := len(handleFunc)

	if n > 1 {
		lastI := n - 1
		return func(ctx context.Context, data Data, next Process) (Data, error) {
			var (
				chainHandler Process
				curI         int
			)
			chainHandler = func(currentCtx context.Context, currentData Data) (Data, error) {
				if curI == lastI {
					return next(currentCtx, data)
				}
				curI++
				data, err := handleFunc[curI](currentCtx, currentData, chainHandler)
				curI--

				return data, err
			}
			return handleFunc[0](ctx, data, chainHandler)
		}
	}

	if n == 1 {
		return handleFunc[0]
	}

	return func(ctx context.Context, data Data, next Process) (Data, error) {
		return next(ctx, data)
	}
}
