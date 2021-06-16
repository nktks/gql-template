package model

import (
    "time"

	"cloud.google.com/go/spanner"
	"github.com/xflagstudio/kangaroo-feed-uni/internal/pkg/util"
)


{{- range $ti, $t  := .Types }}
  {{- if not $t.BuiltIn }}
    {{- if eq $t.Kind "OBJECT" }}
      {{- if and (ne $t.Name "Query") (ne $t.Name "Mutation") (ne $t.Name "Subscription") }}
type {{ $t.Name | title }} struct {
    {{ if eq (foundPK $t.Name $t.Fields) "" }}{{ joinstr (title $t.Name) "Id" }}   string `spanner:"{{ untitle (joinstr $t.Name "Id") }}"`{{ end }}
        {{- range $fi, $f  := $t.Fields }}
          {{- $cfn := ConvertObjectFieldName $f }}
    {{ $f.Name | title }}   {{ GoType $f false }} `{{ if (eq $cfn $f.Name) }}spanner:"{{ $f.Name }}" {{ end }}json:"{{ $f.Name }}"`
          {{- if not (eq $cfn $f.Name) }}
    {{ $cfn | title }}   {{ GoType $f true }} `spanner:"{{ $cfn }}"`
          {{- end }}
        {{- end }}
    {{ if not (exists $t "CreatedAt") }}CreatedAt   time.Time `spanner:"createdAt"`{{ end }}
    {{ if not (exists $t "UpdatedAt") }}UpdatedAt   time.Time `spanner:"updatedAt"`{{ end }}
}

func (m *{{ $t.Name | title }}) SetIdentity() (err error) {
      {{ $pk := foundPK $t.Name $t.Fields }}
	if m.{{ if eq $pk "" }}{{ joinstr (title $t.Name) "Id" }}{{ else }}{{ $pk | title }}{{ end }} == "" {
		m.{{ if eq $pk "" }}{{ joinstr (title $t.Name) "Id" }}{{ else }}{{ $pk | title }}{{ end }}, err = util.NewUUID()
	}
	return
}
      {{- end }}
    {{- end }}
    {{- if eq $t.Kind "ENUM" }}
type {{ $t.Name | title }} string

const (
        {{- range $ei, $enum  := $t.EnumValues }}
	{{ $t.Name | title }}{{ $enum.Name | upperCamel }}         {{ $t.Name | title }} = "{{ $enum.Name | title }}" 
        {{- end }}
)

func (m {{ $t.Name | title }}) EncodeSpanner() (interface{}, error) {
	return string(m), nil
}

func (m *{{ $t.Name | title }}) DecodeSpanner() (interface{}, error) {
	return (*string)(m), nil
}

    {{- end }}
  {{- end }}
{{- end }}
