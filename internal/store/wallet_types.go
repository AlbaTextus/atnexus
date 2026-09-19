package store

import "errors"

// Historical databases retain payment foreign keys until the Credit cutover.
var ErrApplicationHasPayments = errors.New("application has historical payment records")
