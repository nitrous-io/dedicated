package hosts

import (
	"net/http"

	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/user"
)

func Index(c controllers.Helper, u *user.User, w http.ResponseWriter, r *http.Request) (*controllers.JSONResponse, error) {
	db := c.DB()
	hosts, err := host.FindAllByUser(db, u)
	if err != nil {
		return nil, err
	}

	return &controllers.JSONResponse{
		Code: http.StatusOK,
		Data: map[string]interface{}{
			"hosts": hosts,
		},
	}, nil
}
