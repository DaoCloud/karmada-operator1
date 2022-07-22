{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "karmadaoperator.name" -}}
karmada-operator
{{- end }}

{{/*
Return the namespace of karmada-operator install.
*/}}
{{- define "karmadaoperator.namespace" -}}
{{- default .Release.Namespace -}}
{{- end -}}

{{/*
Return the proper karmada-operator controllerManager image name
*/}}
{{- define "karmadaoperator.controllerManager.image" -}}
{{ include "common.images.image" (dict "imageRoot" .Values.controllerManager.image) }}
{{- end -}}


{{/*
Return the proper karmada-operator controllerManager Image Registry Secret Names
*/}}
{{- define "karmadaoperator.controllerManager.imagePullSecrets" -}}
{{ include "common.images.pullSecrets" (dict "images" (list .Values.controllerManager.image)) }}
{{- end -}}


{{- define "karmadaoperator.controllerManager.labels" -}}
{{ $name :=  include "karmadaoperator.name" . }}
{{- if .Values.controllerManager.labels -}}
{{- range $key, $value := .Values.controllerManager.labels -}}
{{ $key }}: {{ $value }}
{{- end -}}
{{- else -}}
app: {{ $name }}-controller-manager
{{- end -}}
{{- end -}}

{{- define "karmadaoperator.controllerManager.podLabels" -}}
{{- if .Values.controllerManager.podLabels }}
{{- range $key, $value := .Values.controllerManager.podLabels}}
{{ $key }}: {{ $value }}
{{- end}}
{{- end }}
{{- end -}}
