package postgres_test

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/go-trainee-techcore/go-market/shared/postgres"
)

func TestConfig_DSN_EscapesPassword(t *testing.T) {
	t.Parallel()

	cfg := postgres.Config{
		Host: "db.internal", Port: 5432, User: "cart",
		Password: "p@ss:w/rd?#", Database: "cart", SSLMode: "disable",
	}

	u, err := url.Parse(cfg.DSN())

	require.NoError(t, err, "спецсимволы в пароле не должны ломать строку подключения")
	pass, _ := u.User.Password()
	assert.Equal(t, "p@ss:w/rd?#", pass)
	assert.Equal(t, "db.internal:5432", u.Host)
	assert.Equal(t, "/cart", u.Path)
	assert.Equal(t, "disable", u.Query().Get("sslmode"))
}
