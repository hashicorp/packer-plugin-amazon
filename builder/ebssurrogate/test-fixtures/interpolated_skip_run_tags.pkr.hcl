source "amazon-ebssurrogate" "test" {
	ami_name = "%s"
	region = "us-east-1"
	instance_type = "m3.medium"
	source_ami = "ami-76b2a71e"
	ssh_username = "ubuntu"
	launch_block_device_mappings {
		device_name = "/dev/xvda"
		delete_on_termination = true
		volume_size = 8
		volume_type = "gp2"
	}
	ami_virtualization_type = "hvm"
	ami_root_device {
		source_device_name = "/dev/xvda"
		device_name = "/dev/sda1"
		delete_on_termination = true
		volume_size = 8
		volume_type = "gp2"
	}
	run_tags = {
    	"simple" = "Simple String"
	}
  skip_ami_run_tags = true
  use_create_image = true

}

build {
	sources = ["amazon-ebssurrogate.test"]
}