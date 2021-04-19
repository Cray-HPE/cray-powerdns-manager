{{- define "cray-powerdns-manager.image-prefix" -}}
    {{ $base := index . "cray-service" }}
    {{- if $base.imagesHost -}}
        {{- printf "%s/" $base.imagesHost -}}
    {{- else -}}
        {{- printf "" -}}
    {{- end -}}
{{- end -}}

{{/*
Helper function to get the proper image tag
*/}}
{{- define "cray-powerdns-manager.imageTag" -}}
{{- default "latest" .Chart.AppVersion -}}
{{- end -}}
