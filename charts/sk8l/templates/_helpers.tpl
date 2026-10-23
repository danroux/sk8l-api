{{/*
securityContext for the pod level.
*/}}
{{- define "securityContext.pod" -}}
  {{- if and (.Values.securityContext) (index .Values.securityContext "pod") }}
      securityContext:
        {{- $tp := typeOf .Values.securityContext.pod }}
        {{- if eq $tp "string" }}
          {{- tpl .Values.securityContext.pod . | nindent 8 }}
        {{- else }}
          {{- toYaml .Values.securityContext.pod | nindent 8 }}
        {{- end }}
  {{- else }}
      securityContext:
        runAsNonRoot: true
  {{- end }}
{{- end -}}

{{/*
securityContext for the container level.
*/}}
{{- define "securityContext.container" -}}
  {{- if and (.Values.securityContext) (index .Values.securityContext "container") }}
          securityContext:
            {{- $tp := typeOf .Values.securityContext.container }}
            {{- if eq $tp "string" }}
              {{- tpl .Values.securityContext.container . | nindent 12 }}
            {{- else }}
              {{- toYaml .Values.securityContext.container | nindent 12 }}
            {{- end }}
  {{- else }}
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            runAsNonRoot: true
  {{- end }}
{{- end -}}

{{/*
Expand the namespace of the release.
*/}}
{{- define "sk8l.namespace" -}}
{{- default .Release.Namespace .Values.namespace.name -}}
{{- end -}}

{{/*
The name of the service account to use.
*/}}
{{- define "sk8l.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
    {{- default "sk8l" .Values.serviceAccount.metadata.name -}}
{{- else -}}
    {{- default "default" .Values.serviceAccount.metadata.name -}}
{{- end -}}
{{- end -}}
