package client

import (
	"context"
	"net/http"
	"strconv"
)

type registerCharacterRequest struct {
	Name   string `json:"name"`
	Realm  string `json:"realm"`
	Region string `json:"region"`
	IsMain bool   `json:"is_main"`
}

// RegisterCharacter adds a character to the actor's guild roster. The Raider.IO lookup
// happens on the service's next worker tick, so the returned character has Synced
// false and no class, spec or ilvl yet.
func (c *Client) RegisterCharacter(ctx context.Context, actor Actor, name, realm, region string, isMain bool) (Character, error) {
	var out Character
	_, err := c.do(ctx, &actor, http.MethodPost,
		"/api/guilds/"+strconv.FormatUint(actor.GuildID, 10)+"/characters",
		registerCharacterRequest{Name: name, Realm: realm, Region: region, IsMain: isMain},
		&out)
	return out, err
}

// GuildCharacters returns every character registered in the actor's guild.
func (c *Client) GuildCharacters(ctx context.Context, actor Actor) ([]Character, error) {
	var out []Character
	_, err := c.do(ctx, &actor, http.MethodGet,
		"/api/guilds/"+strconv.FormatUint(actor.GuildID, 10)+"/characters", nil, &out)
	return out, err
}

// UserCharacters returns one Discord user's characters within the actor's guild. The
// guild comes from the actor's headers, so a user id alone is enough.
func (c *Client) UserCharacters(ctx context.Context, actor Actor, discordID uint64) ([]Character, error) {
	var out []Character
	_, err := c.do(ctx, &actor, http.MethodGet,
		"/api/users/"+strconv.FormatUint(discordID, 10)+"/characters", nil, &out)
	return out, err
}

// CharacterRoles returns a character's current role menu, in priority order. The role
// select needs this before it opens: SetCharacterRoles replaces the whole menu, so a
// select that opens with nothing ticked would silently drop every earlier choice.
func (c *Client) CharacterRoles(ctx context.Context, actor Actor, characterID string) ([]RoleChoice, error) {
	var out []RoleChoice
	_, err := c.do(ctx, &actor, http.MethodGet, "/api/characters/"+characterID+"/roles", nil, &out)
	return out, err
}

type setRolesRequest struct {
	Roles []RoleChoice `json:"roles"`
}

// SetCharacterRoles replaces a character's whole role menu. Self only, so a raid lead
// editing someone else's gets a 403 rather than a silent no-op.
func (c *Client) SetCharacterRoles(ctx context.Context, actor Actor, characterID string, roles []RoleChoice) error {
	_, err := c.do(ctx, &actor, http.MethodPut,
		"/api/characters/"+characterID+"/roles", setRolesRequest{Roles: roles}, nil)
	return err
}

type setMainRequest struct {
	IsMain bool `json:"is_main"`
}

// SetCharacterMain flags which character is the raider's main.
func (c *Client) SetCharacterMain(ctx context.Context, actor Actor, characterID string, isMain bool) (Character, error) {
	var out Character
	_, err := c.do(ctx, &actor, http.MethodPatch,
		"/api/characters/"+characterID, setMainRequest{IsMain: isMain}, &out)
	return out, err
}

// DeleteCharacter unregisters a character. Everything hanging off it cascades, signups
// and comp slots included, so this is for a mistyped registration and not for a raider
// leaving the guild.
func (c *Client) DeleteCharacter(ctx context.Context, actor Actor, characterID string) error {
	_, err := c.do(ctx, &actor, http.MethodDelete, "/api/characters/"+characterID, nil, nil)
	return err
}
