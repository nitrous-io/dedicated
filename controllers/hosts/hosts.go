package hosts

import (
	"net/http"

	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/user"
)

func Index(c controllers.Helper, u *user.User, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	hosts, err := host.FindAllByUser(db, u)
	if err != nil {
		return err
	}
	c.RespondEncodeJSON(w, http.StatusOK, map[string]interface{}{
		"hosts": hosts,
	})
	return nil
}
