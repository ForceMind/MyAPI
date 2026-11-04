package middleware

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/gin-gonic/gin"
)

// A schema is an explicit output contract, never a blacklist of secret names.
// nil accepts scalars only; '*' denotes arrays and '#' numeric-keyed counters.
type managementProjection map[string]managementProjection

func managementScalars(fields string) managementProjection {
	schema := managementProjection{}
	for _, field := range strings.Fields(fields) {
		schema[field] = nil
	}
	return schema
}

func projectManagementJSON(raw json.RawMessage, schema managementProjection) (json.RawMessage, error) {
	raw = bytes.TrimSpace(raw)
	if bytes.Equal(raw, []byte("null")) {
		return raw, nil
	}
	if len(raw) == 0 {
		return nil, errors.New("missing management response")
	}
	if _, numeric := schema["$number"]; numeric {
		var number json.Number
		if raw[0] == '"' || common.Unmarshal(raw, &number) != nil {
			return nil, errors.New("invalid usage number")
		}
		return raw, nil
	}
	if schema == nil {
		if raw[0] == '{' || raw[0] == '[' {
			return nil, errors.New("unexpected management value")
		}
		var scalar any
		if err := common.Unmarshal(raw, &scalar); err != nil {
			return nil, err
		}
		return raw, nil
	}
	if item, ok := schema["*"]; ok {
		var items []json.RawMessage
		if err := common.Unmarshal(raw, &items); err != nil {
			return nil, err
		}
		for i := range items {
			value, err := projectManagementJSON(items[i], item)
			if err != nil {
				return nil, err
			}
			items[i] = value
		}
		return common.Marshal(items)
	}
	var fields map[string]json.RawMessage
	if err := common.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	result := map[string]json.RawMessage{}
	if _, ok := schema["#"]; ok {
		for field, value := range fields {
			if _, err := strconv.Atoi(field); err != nil {
				return nil, err
			}
			var count int64
			if common.Unmarshal(value, &count) != nil {
				return nil, errors.New("invalid counter")
			}
			result[field] = value
		}
	} else {
		for field, child := range schema {
			if field == "usage" {
				if source, present := fields["other"]; present {
					var metadata string
					if common.Unmarshal(source, &metadata) != nil {
						return nil, errors.New("invalid usage metadata")
					}
					if strings.TrimSpace(metadata) != "" {
						value, err := projectManagementJSON([]byte(metadata), child)
						if err != nil {
							return nil, err
						}
						result[field] = value
					}
				}
				continue
			}
			if value, ok := fields[field]; ok {
				projected, err := projectManagementJSON(value, child)
				if err != nil {
					return nil, err
				}
				result[field] = projected
			}
		}
	}
	return common.Marshal(result)
}

func managementResponseSchema(operation string) managementProjection {
	channel := managementScalars("id name type status weight created_time test_time response_time balance balance_updated_time models group used_quota priority auto_ban tag")
	token := managementScalars("id name status created_time accessed_time expired_time remain_quota unlimited_quota model_limits_enabled model_limits used_quota group access_profile_id cross_group_retry quota_version")
	log := managementScalars("id user_id created_at type username token_name model_name quota prompt_tokens completion_tokens use_time is_stream channel channel_name token_id group request_id upstream_request_id")

	usage := managementProjection{}
	for _, field := range strings.Fields("model_ratio group_ratio completion_ratio cache_tokens cache_ratio model_price user_group_ratio frt realtime_cached_tokens cached_text_tokens cached_audio_tokens cached_image_tokens image_input image_output audio_input audio_output text_input text_output audio_ratio audio_completion_ratio cache_creation_tokens cache_creation_ratio cache_creation_tokens_5m cache_creation_ratio_5m cache_creation_tokens_1h cache_creation_ratio_1h") {
		usage[field] = managementProjection{"$number": nil}
	}
	usage["token_usage_evidence"] = managementProjection{"schema": {"$number": nil}, "input_tokens": {"$number": nil}, "output_tokens": {"$number": nil}, "cache_read_tokens": {"$number": nil}}
	for _, field := range strings.Fields("input_tokens_total cache_write_tokens") {
		usage[field] = managementProjection{"$number": nil}
	}
	for _, field := range strings.Fields("usage_accuracy usage_outcome usage_semantic") {
		usage[field] = nil
	}
	log["usage"] = usage
	page := managementScalars("total page page_size")
	switch operation {
	case "channels.list":
		page["items"] = managementProjection{"*": channel}
		page["type_counts"] = managementProjection{"#": nil}
		return page
	case "channels.get":
		return channel
	case "tokens.list":
		page["items"] = managementProjection{"*": token}
		return page
	case "tokens.get":
		return token
	case "logs.list":
		page["items"] = managementProjection{"*": log}
		return page
	case "logs.stats":
		schema := managementScalars("quota rpm tpm")
		schema["token_usage"] = managementScalars("records known_records unknown_records input_tokens output_tokens total_tokens known_input_tokens known_output_tokens known_total_tokens cache_known_records cache_unknown_records cache_read_tokens cache_input_tokens cache_hit_rate")
		return schema
	case "logs.overview":
		item := managementScalars("id created_at model_name username")
		return managementProjection{"requests": {"*": item}, "errors": {"*": item}}
	case "channels.ops":
		return managementScalars("retry_times")
	case "channels.quota_status":
		return managementScalars("enabled interval_seconds max_channels")
	case "channels.quota_history":
		schema := managementScalars("channel_id start end limit granularity requested_granularity timezone_offset raw_observation_limit raw_observations available_points returned_points source_complete points_complete complete truncated metric_type window_type window_seconds source unit currency plan_type")
		point := managementScalars("timestamp observed_at status available used used_source total reset_at sample_count success_count failed_count unsupported_count reset continuity_break consumption rate_per_minute peak_rate_per_minute observed_seconds interval_count gap recovery baseline_change period_start period_end")
		schema["points"] = managementProjection{"*": point}
		schema["current"] = managementScalars("observed_at available used total reset_at status")

		schema["alert"] = managementScalars("enabled status ratio_percent warning_percent critical_percent cooldown_seconds notify_on_recovery")
		schema["identity_quality"] = nil
		schema["series_id"] = nil
		schema["series"] = managementScalars("id metric_type window_type source plan_type unit currency window_seconds")
		quality := managementScalars("sample_count success_count error_count unsupported_count invalid_count reset_boundaries observed_span_seconds span_seconds")
		schema["data_quality"] = quality
		summary := managementScalars("start_available end_available change change_percent minimum maximum drop_rate_per_day forecast_zero_at forecast_confidence")
		value := managementScalars("start end change change_percent minimum maximum samples")
		summary["available"], summary["used"], summary["total"] = value, value, value
		summary["data_quality"] = quality
		summary["consumption"] = managementScalars("observed basis pair_count observed_seconds average_rate_per_minute peak_rate_per_minute peak_rate_observed_at reset_boundaries recovery_count interrupted_count gap_count baseline_change_count unit allocation")
		schema["summary"] = summary
		analysis := managementScalars("default_method rate_window_seconds window_start window_end as_of ewma_half_life_seconds complete")
		method := managementScalars("rate_per_minute rate_per_hour coverage observed_seconds interval_count observed_at")
		method["eta"] = managementScalars("outcome estimated_depletion_at seconds_to_depletion reset_at seconds_until_reset")
		analysis["methods"] = managementProjection{"latest_interval": method, "observed_window": method, "ewma": method}
		schema["analysis"] = analysis
		return schema
	case "channels.set_status":
		return nil

	case "system.instances":
		instance := managementScalars("node_name status stale_after_seconds started_at last_seen_at")
		info := managementScalars("schema_version")
		info["runtime"] = managementScalars("version goos goarch started_at")
		info["role"] = managementScalars("is_master")
		info["resources"] = managementProjection{"cpu": managementScalars("usage_percent"), "memory": managementScalars("usage_percent"), "storage": managementScalars("total_bytes used_bytes free_bytes used_percent")}
		instance["info"] = info
		return managementProjection{"*": instance}
	case "system.self":
		return managementScalars("id username display_name role status group account_tier_id self_use_no_balance usage_policy_revision quota used_quota request_count")
	case "capabilities":
		return managementProjection{"key_id": nil, "scopes": {"*": nil}, "channel_ids": {"*": nil}, "token_ids": {"*": nil}, "expires_at": nil, "operations": {"*": nil}}
	}
	return managementProjection{}
}

type managementResponseWriter struct {
	gin.ResponseWriter
	body     bytes.Buffer
	status   int
	overflow bool
}

func (w *managementResponseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *managementResponseWriter) WriteHeaderNow() {
	if w.status == 0 {
		w.status = http.StatusOK
	}
}
func (w *managementResponseWriter) Write(data []byte) (int, error) {
	w.WriteHeaderNow()
	if w.body.Len()+len(data) > 4*1024*1024 {
		w.overflow = true
		return len(data), nil
	}
	if !w.overflow {
		_, _ = w.body.Write(data)
	}
	return len(data), nil
}
func (w *managementResponseWriter) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
}
func (w *managementResponseWriter) Flush() { w.WriteHeaderNow() }
func (w *managementResponseWriter) Status() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}
func (w *managementResponseWriter) Size() int     { return w.body.Len() }
func (w *managementResponseWriter) Written() bool { return w.status != 0 }

func beginManagementResponse(c *gin.Context) *managementResponseWriter {
	writer := &managementResponseWriter{ResponseWriter: c.Writer}
	c.Writer = writer
	c.Header("Cache-Control", "no-store")
	return writer
}

func finishManagementResponse(c *gin.Context, writer *managementResponseWriter) {
	if writer == nil {
		return
	}
	// Other middleware (including audit) may have wrapped the capture writer.
	c.Writer = writer.ResponseWriter
	var envelope map[string]json.RawMessage
	var success bool
	valid := !writer.overflow && writer.Status() < 400 && common.Unmarshal(writer.body.Bytes(), &envelope) == nil && common.Unmarshal(envelope["success"], &success) == nil && success
	operation := c.GetString("management_operation")
	if c.FullPath() == "/api/management/capabilities" {
		operation = "capabilities"
	}
	if valid {
		result := gin.H{"success": true, "message": ""}
		if data, present := envelope["data"]; present {
			projected, err := projectManagementJSON(data, managementResponseSchema(operation))
			if err != nil {
				valid = false
			} else {
				result["data"] = projected
			}
		}
		if valid {
			for _, field := range strings.Fields("committed cache_pending data_generation published_generation cluster_committed_epoch local_published_epoch cache_enabled") {
				if value, ok := envelope[field]; ok {
					projected, err := projectManagementJSON(value, nil)
					if err != nil {
						valid = false
						break
					}
					result[field] = projected
				}
			}
		}
		if valid {
			c.JSON(writer.Status(), result)
			return
		}
	}
	status := writer.Status()
	if status < 400 {
		status = http.StatusBadGateway
	}
	c.JSON(status, gin.H{"success": false, "code": "MANAGEMENT_RESPONSE_REJECTED", "message": common.TranslateMessage(c, i18n.MsgAuthInsufficientPrivilege)})
}
