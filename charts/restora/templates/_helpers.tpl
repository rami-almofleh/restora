{{/* Common labels */}}
{{- define "restora.labels" -}}
app.kubernetes.io/name: restora
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end }}

{{/* Selector labels for one component (controller or api) */}}
{{- define "restora.selectorLabels" -}}
app.kubernetes.io/name: restora
app.kubernetes.io/instance: {{ .root.Release.Name }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{/* Image reference. Usage: include "restora.image" (dict "root" . "name" .Values.api.imageName) */}}
{{- define "restora.image" -}}
{{- if .root.Values.image.repository -}}
{{ printf "%s/%s:%s" (.root.Values.image.repository | trimSuffix "/") .name .root.Values.image.tag }}
{{- else -}}
{{ printf "%s:%s" .name .root.Values.image.tag }}
{{- end -}}
{{- end }}

{{/* Container security settings shared by all components */}}
{{- define "restora.containerSecurityContext" -}}
allowPrivilegeEscalation: false
readOnlyRootFilesystem: true
runAsNonRoot: true
runAsUser: 65532
runAsGroup: 65532
capabilities:
  drop: ["ALL"]
seccompProfile:
  type: RuntimeDefault
{{- end }}
