package controllers

import (
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/db"
)

type Helper interface {
	Root() string
	Logger() *logger.Logger
	DB() *db.Database
}
