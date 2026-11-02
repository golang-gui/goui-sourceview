# Original HCL / Terraform sample for SourceView.
variable "name" {
  type    = string
  default = "SourceView"
}

locals {
  greeting = "Hello, ${var.name}!"
  document = <<-TEXT
    ${local.greeting}
    %{ for item in ["native", "code", "editor"] ~}
    - ${item}
    %{ endfor ~}
    Literal template markers: $${name} and %%{if}.
    TEXT
}

output "message" {
  value = local.document
}
