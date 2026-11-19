package httpapi

import (
	"github.com/AlbaTextus/atnexus/internal/apibase"
	"github.com/AlbaTextus/atnexus/modules/mail"
)

var writeJSON = apibase.WriteJSON
var readAccountJSON = apibase.ReadJSON
var isAdmin = apibase.IsAdmin
var normalizeEmail = mail.NormalizeAddress
var validEmail = mail.ValidAddress
