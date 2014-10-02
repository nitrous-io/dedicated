package controllers_test

import (
	"database/sql"
	"net/http"

	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/pkg/httperror"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("controllers", func() {
	Describe("NewFreighterClient", func() {
		It("returns HTTP 400 when ip address is missing", func() {
			_, err := controllers.NewFreighterClient(&sqlx.DB{}, &host.Host{})
			Expect(err).To(Equal(controllers.ErrHostIPUnknown))
			Expect(err.(*httperror.Error).Code).To(Equal(http.StatusBadRequest))
		})

		It("returns HTTP 503 when state is not running", func() {
			_, err := controllers.NewFreighterClient(&sqlx.DB{}, &host.Host{
				IPAddress: sql.NullString{"1.1.1.1", true},
			})
			Expect(err).To(Equal(controllers.ErrHostNotRunning))
			Expect(err.(*httperror.Error).Code).To(Equal(http.StatusServiceUnavailable))
		})
	})
})
