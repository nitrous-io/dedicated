package containers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

type HostWithUser struct {
	host.Host
	UserUsername string `db:"user_username"`
	UserName     string `db:"user_name"`
	GravatarHash string `db:"user_hash"`
}

type HostUserContainerResult struct {
	Hosts          []*HostWithUser
	HostContainers map[int64][]*container.Container
}

func Shared(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	res, err := fetchContainersWithHosts(db, env.User.ID)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	if res == nil || len(res.Hosts) == 0 {
		env.Data = map[string]interface{}{
			"containers": []interface{}{},
		}
		return nil
	}

	hosts := res.Hosts
	containerHostMap := res.HostContainers

	// produce an array of type asserted Hosts
	// while using the same loop to build a map of containers to hosts
	// to fetch for serialization purposes
	hostMap := make(map[int64]*HostWithUser)
	hostsOnly := make([]*host.Host, len(hosts))
	for i, _ := range hosts {
		hostsOnly[i] = &(hosts[i].Host)
		hostMap[hosts[i].ID] = hosts[i]
	}

	env.Data = map[string]interface{}{}
	serContainers := listContainers(env, w, r, hostsOnly, containerHostMap, true)

	// extended the initial serialized container with partial host information
	serContainerUsers := make([]*serializers.SharedContainer, len(serContainers))
	for i, c := range serContainers {
		hU, ok := hostMap[c.HostID]
		if !ok {
			continue
		}
		serContainerUsers[i] = &serializers.SharedContainer{
			*c,
			hU.UserUsername,
			hU.UserName,
			hU.GravatarHash,
			serializers.SerializeHost(&hU.Host),
		}
	}
	env.Data["containers"] = serContainerUsers
	return nil
}

// look for the containers shared with a user and then collect the hosts based on this
func fetchContainersWithHosts(db *db.Database, userID int64) (*HostUserContainerResult, error) {
	hosts := []*HostWithUser{}

	containers, err := container.FindAllShared(db, userID)
	if err != nil {
		return nil, err
	}

	if len(containers) == 0 {
		return nil, nil
	}

	containerHostMap := make(map[int64][]*container.Container)
	hostIDs := []string{}

	for _, con := range containers {
		strHID := strconv.FormatInt(con.HostID, 10)
		hostIDs = append(hostIDs, strHID)
		containerHostMap[con.HostID] = append(containerHostMap[con.HostID], con)
	}

	baseQuery := fmt.Sprintf(`
    SELECT
    hosts.*,
    users.name user_name,
    users.username user_username,
    md5(users.email) user_hash
    FROM hosts
    INNER JOIN users ON users.id = hosts.user_id
    WHERE hosts.id IN (%s)
    AND terminated_at IS NULL
    AND preprovisioned = FALSE
    AND state NOT IN ($1, $2)
    ORDER BY id;`,
		strings.Join(hostIDs, ", "))
	err = db.Select(&hosts, baseQuery, host.StateOrphan, host.StateMigrating)
	if err != nil {
		return nil, err
	}

	return &HostUserContainerResult{
		Hosts:          hosts,
		HostContainers: containerHostMap,
	}, nil
}
