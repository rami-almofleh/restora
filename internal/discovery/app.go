package discovery

import (
	"sort"
	"time"
)

// app collects everything found in one namespace.
type app struct {
	name       string
	labels     map[string]string
	createdAt  time.Time
	workloads  []Workload
	volumes    []Volume
	configMaps []string
	secrets    []string
}

func (a *app) summary() Summary {
	return Summary{
		Name:       a.name,
		Health:     a.health(),
		Engines:    a.engines(),
		Workloads:  len(a.workloads),
		Volumes:    len(a.volumes),
		ConfigMaps: len(a.configMaps),
		Secrets:    len(a.secrets),
		CreatedAt:  a.createdAt.UTC(),
	}
}

func (a *app) detail() Detail {
	workloads := append([]Workload{}, a.workloads...)
	for i := range workloads {
		workloads[i].Engine = detectEngine(workloads[i].Images)
		if workloads[i].Images == nil {
			workloads[i].Images = []string{}
		}
	}
	sort.Slice(workloads, func(i, j int) bool {
		if workloads[i].Kind != workloads[j].Kind {
			return workloads[i].Kind < workloads[j].Kind
		}
		return workloads[i].Name < workloads[j].Name
	})
	volumes := append([]Volume{}, a.volumes...)
	sort.Slice(volumes, func(i, j int) bool { return volumes[i].Name < volumes[j].Name })

	return Detail{
		Summary:        a.summary(),
		WorkloadItems:  workloads,
		VolumeItems:    volumes,
		ConfigMapNames: sortedCopy(a.configMaps),
		SecretNames:    sortedCopy(a.secrets),
	}
}

func (a *app) health() Health {
	desiredTotal := int32(0)
	degraded := false
	for _, w := range a.workloads {
		desiredTotal += w.Desired
		if w.Ready < w.Desired {
			degraded = true
		}
	}
	switch {
	case desiredTotal == 0:
		return HealthStopped
	case degraded:
		return HealthDegraded
	default:
		return HealthRunning
	}
}

// engines lists each detected product once, sorted.
func (a *app) engines() []string {
	seen := map[string]bool{}
	engines := []string{}
	for _, w := range a.workloads {
		if e := detectEngine(w.Images); e != "" && !seen[e] {
			seen[e] = true
			engines = append(engines, e)
		}
	}
	sort.Strings(engines)
	return engines
}

func sortedCopy(in []string) []string {
	out := append([]string{}, in...)
	sort.Strings(out)
	return out
}
