package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"

	"github.com/gin-gonic/gin"
)

func KlingRequestConvert() func(c *gin.Context) {
	return func(c *gin.Context) {
		var originalReq map[string]interface{}
		if err := common.UnmarshalBodyReusable(c, &originalReq); err != nil {
			c.Next()
			return
		}

		// Support both model_name and model fields
		model, _ := originalReq["model_name"].(string)
		if model == "" {
			model, _ = originalReq["model"].(string)
		}
		prompt, _ := originalReq["prompt"].(string)

		unifiedReq := map[string]interface{}{
			"model":    model,
			"prompt":   prompt,
			"metadata": originalReq,
		}

		jsonData, err := json.Marshal(unifiedReq)
		if err != nil {
			c.Next()
			return
		}

		// Invalidate stale body cache so Distribute reads the rewritten body.
		if oldStorage, exists := c.Get(common.KeyBodyStorage); exists && oldStorage != nil {
			if bs, ok := oldStorage.(common.BodyStorage); ok {
				_ = bs.Close()
			}
		}
		c.Set(common.KeyBodyStorage, nil)

		// Rewrite request body and path
		c.Request.Body = io.NopCloser(bytes.NewBuffer(jsonData))
		c.Request.URL.Path = "/v1/video/generations"
		if image, ok := originalReq["image"]; !ok || image == "" {
			c.Set("action", constant.TaskActionTextGenerate)
		}
		c.Set(common.KeyRequestBody, jsonData)
		c.Next()
	}
}

// KlingV3RequestConvert handles the Kling v3 native API format:
//
//	POST /kling/text-to-video/kling-3.0-turbo
//	POST /kling/image-to-video/kling-3.0-turbo
//
// Request body (native v3 format):
//
//	{
//	  "prompt":  "...",
//	  "image":   "...",          (I2V only)
//	  "options": { "callback_url":"...", "watermark_info":{...}, "external_task_id":"..." },
//	  "settings":{ "duration":5, "resolution":"720p", "aspect_ratio":"16:9" }
//	}
//
// Converts to the unified TaskSubmitReq and rewrites the path to /v1/video/generations.
// The full original body is kept in "metadata" so the kling adaptor's convertToV3RequestPayload
// can pick up options/settings via UnmarshalMetadata.
func KlingV3RequestConvert() func(c *gin.Context) {
	return func(c *gin.Context) {
		// Model ID is the last path segment, e.g. "kling-3.0-turbo".
		// c.Param works when the route has been matched, but fall back to
		// manual path parsing in case group-level middleware runs before
		// Gin binds the named parameters.
		modelId := c.Param("model_id")
		if modelId == "" {
			// /kling/text-to-video/kling-3.0-turbo → last segment
			p := c.Request.URL.Path
			if idx := strings.LastIndex(p, "/"); idx >= 0 {
				modelId = p[idx+1:]
			}
		}
		internalModel := klingV3APIToInternal(modelId)

		var originalReq map[string]interface{}
		if err := common.UnmarshalBodyReusable(c, &originalReq); err != nil {
			c.Next()
			return
		}

		// --- Top-level fields (T2V style) ---
		prompt, _ := originalReq["prompt"].(string)
		image, _ := originalReq["image"].(string)

		// --- contents array (I2V style) ---
		// contents: [{type:"prompt",text:"..."},{type:"first_frame",url:"..."}]
		isI2V := image != ""
		if rawContents, ok := originalReq["contents"].([]interface{}); ok {
			for _, item := range rawContents {
				m, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				switch m["type"] {
				case "prompt":
					if prompt == "" {
						prompt, _ = m["text"].(string)
					}
				case "first_frame":
					if image == "" {
						image, _ = m["url"].(string)
					}
					isI2V = true
				}
			}
		}

		// --- settings ---
		var duration int
		var resolution, size string
		if settings, ok := originalReq["settings"].(map[string]interface{}); ok {
			if d, ok := settings["duration"].(float64); ok {
				duration = int(d)
			}
			resolution, _ = settings["resolution"].(string)
			if ar, ok := settings["aspect_ratio"].(string); ok {
				size = klingAspectRatioToSize(ar)
			}
		}

		unifiedReq := map[string]interface{}{
			"model":      internalModel,
			"prompt":     prompt,
			"image":      image,  // adaptor uses this to build contents array
			"duration":   duration,
			"resolution": resolution,
			"size":       size,
			"metadata":   originalReq, // full native body; adaptor UnmarshalMetadata picks up contents/options/settings
		}

		jsonData, err := json.Marshal(unifiedReq)
		if err != nil {
			c.Next()
			return
		}

		// Invalidate the stale body cache that UnmarshalBodyReusable created from
		// the original request. Without this, getModelFromJSONBody in Distribute
		// would read the old body (which has no "model" field) and fail.
		if oldStorage, exists := c.Get(common.KeyBodyStorage); exists && oldStorage != nil {
			if bs, ok := oldStorage.(common.BodyStorage); ok {
				_ = bs.Close()
			}
		}
		c.Set(common.KeyBodyStorage, nil)

		c.Request.Body = io.NopCloser(bytes.NewBuffer(jsonData))
		c.Request.URL.Path = "/v1/video/generations"
		if !isI2V {
			c.Set("action", constant.TaskActionTextGenerate)
		}
		c.Set(common.KeyRequestBody, jsonData)
		c.Next()
	}
}

// klingV3APIToInternal maps a Kling v3 API model identifier (as used in the URL path)
// to the internal new-api model name used for channel routing and billing.
//
//	kling-3.0-turbo → kling-v3-turbo
func klingV3APIToInternal(apiModelId string) string {
	switch strings.ToLower(strings.TrimSpace(apiModelId)) {
	case "kling-3.0-turbo":
		return "kling-v3-turbo"
	default:
		return apiModelId
	}
}

// klingAspectRatioToSize converts a Kling aspect_ratio string to a "WxH" size string
// used by the standard TaskSubmitReq.Size field.
//
//	"16:9" → "1280x720"
//	"9:16" → "720x1280"
//	"1:1"  → "1024x1024"
func klingAspectRatioToSize(ar string) string {
	switch ar {
	case "16:9":
		return "1280x720"
	case "9:16":
		return "720x1280"
	case "1:1":
		return "1024x1024"
	default:
		return ""
	}
}
