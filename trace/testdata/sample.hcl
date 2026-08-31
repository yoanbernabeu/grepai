resource "aws_instance" "web" {
    ami = "ami-123"
}
variable "region" { default = "us-east-1" }
locals { name = "foo" }
