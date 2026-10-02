{{- define "todo.labels" -}}
app.kubernetes.io/name: todo
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end -}}

{{/* Селектор компонента: include "todo.selector" (dict "ctx" . "component" "backend") */}}
{{- define "todo.selector" -}}
app.kubernetes.io/name: todo
app.kubernetes.io/instance: {{ .ctx.Release.Name }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{- define "todo.image" -}}
{{- required "image.tag задаётся в values-<env>.yaml" .Values.image.tag | printf "%s:%s" .Values.image.repository -}}
{{- end -}}

{{/* Хост registry — из image.repository (192.168.0.21:5050/... -> 192.168.0.21:5050) */}}
{{- define "todo.registryHost" -}}
{{- regexFind "^[^/]+" .Values.image.repository -}}
{{- end -}}

{{- define "todo.vaultPath" -}}
{{- required "vault.path задаётся в values-<env>.yaml (например secret/dev)" .Values.vault.path -}}
{{- end -}}

{{/* Пути Ingress: всё на Service backend */}}
{{- define "todo.ingressPaths" -}}
paths:
  - path: /
    pathType: Prefix
    backend:
      service:
        name: backend
        port:
          name: http
{{- end }}

{{/* Размещение подов приложения: nodeSelector и tolerations из .Values.placement */}}
{{- define "todo.placement" -}}
{{- with .Values.placement.nodeSelector }}
nodeSelector:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- with .Values.placement.tolerations }}
tolerations:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- end }}
