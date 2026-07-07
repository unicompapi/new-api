package ali

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

const videoEditSucceededResponse = `{
	"request_id": "c11018a8-3f83-9591-a636-xxxxxx",
	"output": {
		"task_id": "051c7b40-b2c5-4341-aee4-xxxxxx",
		"task_status": "SUCCEEDED",
		"submit_time": "2026-04-26 14:13:14.373",
		"scheduled_time": "2026-04-26 14:13:14.419",
		"end_time": "2026-04-26 14:14:13.679",
		"orig_prompt": "让视频中的马头人身角色穿上图片中的条纹毛衣",
		"video_url": "https://dashscope-result.oss-cn-beijing.aliyuncs.com/xxxx.mp4"
	},
	"usage": {
		"duration": 13.24,
		"input_video_duration": 6.62,
		"output_video_duration": 6.62,
		"video_count": 1,
		"SR": 720
	}
}`

func TestParseTaskResultVideoEditWithFloatUsage(t *testing.T) {
	adaptor := &TaskAdaptor{}
	taskInfo, err := adaptor.ParseTaskResult([]byte(videoEditSucceededResponse))
	require.NoError(t, err)
	require.Equal(t, model.TaskStatusSuccess, taskInfo.Status)
	require.Equal(t, "https://dashscope-result.oss-cn-beijing.aliyuncs.com/xxxx.mp4", taskInfo.Url)
}

func TestConvertToOpenAIVideoVideoEditWithFloatUsage(t *testing.T) {
	adaptor := &TaskAdaptor{}
	task := &model.Task{
		TaskID: "task_test",
		Data:   []byte(videoEditSucceededResponse),
		PrivateData: model.TaskPrivateData{
			ResultURL: "https://dashscope-result.oss-cn-beijing.aliyuncs.com/xxxx.mp4",
		},
	}
	body, err := adaptor.ConvertToOpenAIVideo(task)
	require.NoError(t, err)
	require.Contains(t, string(body), "https://dashscope-result.oss-cn-beijing.aliyuncs.com/xxxx.mp4")
}
