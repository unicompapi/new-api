package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestUserListExcludesDeletedAndFiltersPhone(t *testing.T) {
	truncateTables(t)

	phone := "+8613800138000"
	legacy := &User{Username: "legacy-user", AffCode: "legacy", Status: common.UserStatusEnabled}
	mobile := &User{Username: "mobile-user", AffCode: "mobile", Phone: &phone, Status: common.UserStatusEnabled}
	deleted := &User{Username: "deleted-user", AffCode: "deleted", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(legacy).Error)
	require.NoError(t, DB.Create(mobile).Error)
	require.NoError(t, DB.Create(deleted).Error)
	require.NoError(t, DB.Delete(deleted).Error)

	users, total, err := GetAllUsers(&common.PageInfo{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, users, 2)

	users, total, err = SearchUsers("13800138000", "", nil, nil, nil, 0, 20)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, mobile.Id, users[0].Id)

	bound := false
	users, total, err = SearchUsers("", "", nil, nil, &bound, 0, 20)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, legacy.Id, users[0].Id)
}
