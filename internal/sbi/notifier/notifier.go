package notifier

import nef_context "github.com/free5gc/nef/internal/context"

type Notifier struct {
	PfdChangeNotifier *PfdChangeNotifier
}

func NewNotifier(_nefCtx *nef_context.NefContext) (*Notifier, error) {
	var err error
	n := &Notifier{}
	if n.PfdChangeNotifier, err = NewPfdChangeNotifier(_nefCtx); err != nil {
		return nil, err
	}
	return n, nil
}
