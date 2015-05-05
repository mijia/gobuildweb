package main

import (
	"fmt"
	"os"
	"strings"
)

func mergeEnv(newEnvs map[string]string) []string {
	baseEnv := os.Environ()
	envs := make(map[string]string)
	for _, env := range baseEnv {
		splits := strings.SplitN(env, "=", 2)
		envs[splits[0]] = splits[1]
	}
	for envKey, envValue := range newEnvs {
		envs[envKey] = envValue
	}
	merged := make([]string, 0, len(envs))
	for key, value := range envs {
		merged = append(merged, fmt.Sprintf("%s=%s", key, value))
	}
	return merged
}
