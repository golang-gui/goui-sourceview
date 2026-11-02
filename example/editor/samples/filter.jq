# Original jq sample for SourceView.
def enabled: select(.enabled == true);
.languages
| map(enabled)
| .[]
| .name as $name
| {name: $name, label: "Language: \($name)", length: ($name | length)}
