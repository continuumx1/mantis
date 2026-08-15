{{/*
Chart name, truncated to fit Kubernetes' 63-char name limits once suffixes
(e.g. "-engine", "-web") are appended.
*/}}
{{- define "mantis.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 55 | trimSuffix "-" -}}
{{- end -}}

{{/*
Full release name, following the standard chart convention: if the release
name already contains the chart name, don't repeat it.
*/}}
{{- define "mantis.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 55 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 55 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 55 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "mantis.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* Labels shared by every resource this chart creates. */}}
{{- define "mantis.labels" -}}
helm.sh/chart: {{ include "mantis.chart" . }}
{{ include "mantis.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "mantis.selectorLabels" -}}
app.kubernetes.io/name: {{ include "mantis.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/* Per-component (engine/web) names and labels — "instance" being just one of the two services. */}}
{{- define "mantis.engine.fullname" -}}
{{- printf "%s-engine" (include "mantis.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "mantis.web.fullname" -}}
{{- printf "%s-web" (include "mantis.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "mantis.engine.selectorLabels" -}}
{{ include "mantis.selectorLabels" . }}
app.kubernetes.io/component: engine
{{- end -}}

{{- define "mantis.web.selectorLabels" -}}
{{ include "mantis.selectorLabels" . }}
app.kubernetes.io/component: web
{{- end -}}

{{- define "mantis.engine.labels" -}}
{{ include "mantis.labels" . }}
app.kubernetes.io/component: engine
{{- end -}}

{{- define "mantis.web.labels" -}}
{{ include "mantis.labels" . }}
app.kubernetes.io/component: web
{{- end -}}

{{/* The ServiceAccount name mantis-engine's Deployment runs under. */}}
{{- define "mantis.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "mantis.engine.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{/*
Image references. Both services publish from one repository, distinguished
by a "-engine" / "-web" tag suffix (e.g. cx1tech/mantis:0.1.0-preview.1-engine)
— see values.yaml's image.* comment for why.
*/}}
{{- define "mantis.engine.image" -}}
{{- printf "%s:%s-engine" .Values.image.repository (.Values.image.tag | default .Chart.AppVersion) -}}
{{- end -}}

{{- define "mantis.web.image" -}}
{{- printf "%s:%s-web" .Values.image.repository (.Values.image.tag | default .Chart.AppVersion) -}}
{{- end -}}
