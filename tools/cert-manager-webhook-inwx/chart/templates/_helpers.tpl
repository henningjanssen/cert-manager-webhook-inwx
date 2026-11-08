{{/*
Expand the name of the chart.
*/}}
{{- define "cert-manager-webhook-inwx.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "cert-manager-webhook-inwx.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart label.
*/}}
{{- define "cert-manager-webhook-inwx.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels.
*/}}
{{- define "cert-manager-webhook-inwx.labels" -}}
helm.sh/chart: {{ include "cert-manager-webhook-inwx.chart" . }}
{{ include "cert-manager-webhook-inwx.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels.
*/}}
{{- define "cert-manager-webhook-inwx.selectorLabels" -}}
app.kubernetes.io/name: {{ include "cert-manager-webhook-inwx.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Service account name.
Uses serviceAccount.name when set, otherwise falls back to the fullname.
When serviceAccount.create is false and name is empty, "default" is used.
*/}}
{{- define "cert-manager-webhook-inwx.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
  {{- default (include "cert-manager-webhook-inwx.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
  {{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end }}

{{/*
Return the full container image reference.
Prefers digest over tag when both are set, which uniquely identifies the image
and makes the reference fully immutable.
Usage: {{ include "cert-manager-webhook-inwx.image" . }}
*/}}
{{- define "cert-manager-webhook-inwx.image" -}}
{{- $repo := .Values.image.repository -}}
{{- if .Values.image.digest -}}
{{- printf "%s@%s" $repo .Values.image.digest -}}
{{- else -}}
{{- printf "%s:%s" $repo (.Values.image.tag | default .Chart.AppVersion) -}}
{{- end -}}
{{- end }}
