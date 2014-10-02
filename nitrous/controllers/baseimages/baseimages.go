package baseimages

import (
	"net/http"

	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/baseimage"
	"github.com/nitrous-io/nitrous/serializers"
)

func Index(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	imgs, err := baseimage.All(env.DB)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"base_images": serializers.SerializeBaseImages(imgs),
	}
	return nil
}
