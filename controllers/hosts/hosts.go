package hosts

import (
	"errors"
	"net/http"

	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/middlewares"
	"github.com/nitrous-io/nitrous/models/host"
)

func Index(env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()

	u := env.User
	if u == nil {
		return errors.New("user is nil")
	}

	hosts, err := host.FindAllByUser(db, u)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"hosts": hosts,
	}
	return nil
}
