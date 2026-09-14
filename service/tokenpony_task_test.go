package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenPonyTaskSurvivesProcessRestartQuery(t *testing.T) {
	truncate(t)
	task := &model.Task{
		TaskID:     "task_restart1",
		Platform:   constant.TaskPlatform("openai"),
		ChannelId:  71,
		Status:     model.TaskStatusInProgress,
		Progress:   "30%",
		SubmitTime: time.Now().Unix(),
		Data:       json.RawMessage(`{"status":"RUNNING"}`),
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "tokenpony-upstream-1",
			LastPolledAt:   123,
			BillingContext: &model.TaskBillingContext{ModelRatio: 1.5, GroupRatio: 2},
		},
	}
	require.NoError(t, model.DB.Create(task).Error)

	// The polling loop rebuilds its in-memory maps from this DB query after every process start.
	tasks := model.GetAllUnFinishSyncTasks(100)
	require.Len(t, tasks, 1)
	assert.Equal(t, "tokenpony-upstream-1", tasks[0].GetUpstreamTaskID())
	assert.EqualValues(t, 123, tasks[0].PrivateData.LastPolledAt)
	require.NotNil(t, tasks[0].PrivateData.BillingContext)
	assert.Equal(t, 1.5, tasks[0].PrivateData.BillingContext.ModelRatio)
}

func TestTokenPonyRepeatedTerminalPollSettlesOnlyOnce(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	const userID, tokenID, channelID = 72, 72, 72
	const initialQuota, tokenRemain, preConsumed, actualQuota = 10000, 7000, 5000, 3000
	seedUser(t, userID, initialQuota)
	seedToken(t, tokenID, userID, "sk-tokenpony-cas", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.TaskID = "task_tokenponyonce"
	require.NoError(t, model.DB.Create(task).Error)

	var pollerA, pollerB model.Task
	require.NoError(t, model.DB.First(&pollerA, task.ID).Error)
	require.NoError(t, model.DB.First(&pollerB, task.ID).Error)
	simulatePollBilling(ctx, &pollerA, model.TaskStatusSuccess, actualQuota)
	simulatePollBilling(ctx, &pollerB, model.TaskStatusSuccess, actualQuota)

	assert.Equal(t, initialQuota+(preConsumed-actualQuota), getUserQuota(t, userID))
	assert.Equal(t, tokenRemain+(preConsumed-actualQuota), getTokenRemainQuota(t, tokenID))
	assert.Equal(t, int64(1), countLogs(t))
}
