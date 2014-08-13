package controllers

import (
	"net/http"

	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/db"
)

type Helper interface {
	Root() string
	Logger() *logger.Logger
	DB() *db.Database

	RenderHTML(w http.ResponseWriter, status int, html []byte)
	RespondJSON(w http.ResponseWriter, status int, data []byte)
	RespondEncodeJSON(w http.ResponseWriter, status int, data map[string]interface{})
}
