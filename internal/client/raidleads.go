package client

import (
	"context"
	"net/http"
	"strconv"
)

type setRaidLeadRolesRequest struct {
	RoleIDs []string `json:"role_ids"`
}

// RaidLeadRoles returns which Discord roles the guild treats as raid lead. Guild admin
// only.
func (c *Client) RaidLeadRoles(ctx context.Context, actor Actor) (RaidLeadRoles, error) {
	var out RaidLeadRoles
	_, err := c.do(ctx, &actor, http.MethodGet,
		"/api/guilds/"+strconv.FormatUint(actor.GuildID, 10)+"/raid-lead-roles", nil, &out)
	return out, err
}

// SetRaidLeadRoles replaces the guild's raid-lead role mapping. Guild admin only.
//
// A guild with no mapping treats Discord admins as raid leads and nobody else, so a
// fresh install is usable before anyone visits the dashboard.
func (c *Client) SetRaidLeadRoles(ctx context.Context, actor Actor, roleIDs []uint64) (RaidLeadRoles, error) {
	ids := make([]string, len(roleIDs))
	for i, id := range roleIDs {
		ids[i] = strconv.FormatUint(id, 10)
	}

	var out RaidLeadRoles
	_, err := c.do(ctx, &actor, http.MethodPut,
		"/api/guilds/"+strconv.FormatUint(actor.GuildID, 10)+"/raid-lead-roles",
		setRaidLeadRolesRequest{RoleIDs: ids}, &out)
	return out, err
}
