package tagteam

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const timeoutCalibrationArtifact = "timeout-calibration.json"

type TimeoutCalibration struct {
	SchemaVersion        int       `json:"schema_version"`
	Adapter              string    `json:"adapter"`
	Model                string    `json:"model,omitempty"`
	AdapterVersion       string    `json:"adapter_version,omitempty"`
	Requested            string    `json:"requested"`
	NativeDefault        string    `json:"native_default,omitempty"`
	EncodedTimeout       string    `json:"encoded_timeout,omitempty"`
	LearnedCap           string    `json:"learned_cap,omitempty"`
	Effective            string    `json:"effective"`
	PackageBudgetSeconds int64     `json:"package_budget_seconds"`
	Source               string    `json:"source"`
	Warning              string    `json:"warning,omitempty"`
	CalibratedAt         time.Time `json:"calibrated_at"`
}

type timeoutObservation struct {
	Adapter        string    `json:"adapter"`
	Model          string    `json:"model,omitempty"`
	AdapterVersion string    `json:"adapter_version,omitempty"`
	Duration       string    `json:"duration"`
	ObservedAt     time.Time `json:"observed_at"`
}

type timeoutHistory struct {
	SchemaVersion int                  `json:"schema_version"`
	Observations  []timeoutObservation `json:"observations"`
}

func calibrateTimeout(ctx context.Context, adapter Adapter, role Role, req Request) (TimeoutCalibration, time.Duration) {
	requested := req.Timeout
	if requested <= 0 {
		requested = 15 * time.Minute
	}
	calibration := TimeoutCalibration{
		SchemaVersion: ArtifactSchemaVersion,
		Adapter:       adapter.ID(),
		Model:         req.Model,
		Requested:     requested.String(),
		Effective:     requested.String(),
		Source:        "tagteam_deadline",
		CalibratedAt:  time.Now().UTC(),
	}
	if info, err := adapter.Detect(ctx); err == nil {
		calibration.AdapterVersion = info.Version
	}
	spec, buildErr := adapter.BuildCmd(role, req)
	if buildErr == nil && spec != nil {
		if encoded := timeoutArgument(spec.Argv, "--print-timeout"); encoded > 0 {
			calibration.EncodedTimeout = encoded.String()
			calibration.Source = "native_argument"
		}
	}
	if adapter.ID() == "agy" {
		if nativeDefault := agyNativeDefaultTimeout(ctx); nativeDefault > 0 {
			calibration.NativeDefault = nativeDefault.String()
			if calibration.EncodedTimeout == "" && nativeDefault < requested {
				requested = nativeDefault
				calibration.Source = "native_default"
			}
		}
	}
	if learned := learnedTimeoutCap(req.RunDir, calibration); learned > 0 && learned < requested {
		requested = learned
		calibration.LearnedCap = learned.String()
		calibration.Source = "historical_cap"
	}
	calibration.Effective = requested.String()
	calibration.PackageBudgetSeconds = int64(math.Floor(requested.Seconds() * 0.8))
	configured, _ := time.ParseDuration(calibration.Requested)
	if configured != requested {
		calibration.Warning = fmt.Sprintf("requested timeout %s constrained to effective timeout %s", configured, requested)
	}
	persistTimeoutCalibration(req.RunDir, calibration)
	return calibration, requested
}

func timeoutArgument(argv []string, name string) time.Duration {
	for index, arg := range argv {
		if arg == name && index+1 < len(argv) {
			duration, _ := time.ParseDuration(argv[index+1])
			return duration
		}
		if strings.HasPrefix(arg, name+"=") {
			duration, _ := time.ParseDuration(strings.TrimPrefix(arg, name+"="))
			return duration
		}
	}
	return 0
}

func agyNativeDefaultTimeout(ctx context.Context) time.Duration {
	cmd := exec.CommandContext(ctx, "agy", "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(output), "\n") {
		if !strings.Contains(line, "--print-timeout") || !strings.Contains(line, "default ") {
			continue
		}
		raw := line[strings.LastIndex(line, "default ")+len("default "):]
		raw = strings.Trim(raw, " ()\t\r")
		if duration, err := time.ParseDuration(raw); err == nil {
			return duration
		}
	}
	return 0
}

func timeoutHistoryPath(runDir string) string {
	if runDir == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(filepath.Dir(runDir)), "adapter-timeout-history.json")
}

func learnedTimeoutCap(runDir string, calibration TimeoutCalibration) time.Duration {
	path := timeoutHistoryPath(runDir)
	if path == "" {
		return 0
	}
	var history timeoutHistory
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &history) != nil {
		return 0
	}
	matches := []time.Duration{}
	for _, observation := range history.Observations {
		if observation.Adapter != calibration.Adapter || observation.Model != calibration.Model || observation.AdapterVersion != calibration.AdapterVersion {
			continue
		}
		if duration, err := time.ParseDuration(observation.Duration); err == nil {
			matches = append(matches, duration)
		}
	}
	if len(matches) < 2 {
		return 0
	}
	first, second := matches[len(matches)-2], matches[len(matches)-1]
	larger := math.Max(first.Seconds(), second.Seconds())
	if larger == 0 || math.Abs(first.Seconds()-second.Seconds())/larger > 0.10 {
		return 0
	}
	if first < second {
		return first
	}
	return second
}

func recordTimeoutObservation(runDir string, calibration TimeoutCalibration, elapsed time.Duration) {
	path := timeoutHistoryPath(runDir)
	if path == "" {
		return
	}
	history := timeoutHistory{SchemaVersion: ArtifactSchemaVersion}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &history)
	}
	history.SchemaVersion = ArtifactSchemaVersion
	history.Observations = append(history.Observations, timeoutObservation{
		Adapter:        calibration.Adapter,
		Model:          calibration.Model,
		AdapterVersion: calibration.AdapterVersion,
		Duration:       elapsed.Round(time.Second).String(),
		ObservedAt:     time.Now().UTC(),
	})
	if len(history.Observations) > 100 {
		history.Observations = history.Observations[len(history.Observations)-100:]
	}
	_ = writeJSONWithNewline(path, history)
}

func persistTimeoutCalibration(runDir string, calibration TimeoutCalibration) {
	if runDir == "" {
		return
	}
	path := filepath.Join(runDir, timeoutCalibrationArtifact)
	all := map[string]TimeoutCalibration{}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &all)
	}
	key := calibration.Adapter + ":" + calibration.Model
	all[key] = calibration
	_ = writeJSONWithNewline(path, all)
}
