variable "ci_workspace_bins" {
  type = string
  default = "./linux"
}

variable "lotus_network" {
  type = string
  default = "mainnet"
}

variable "git_tag" {
  type = string
  default = ""
}

locals {
  timestamp = regex_replace(timestamp(), "[- TZ:]", "")
} 

source "amazon-ebs" "lotus" {
  ami_name      = "lotus-${var.lotus_network}-${var.git_tag}-${local.timestamp}"
  ami_regions = [
    "ap-east-1",
    "ap-northeast-1",
    "ap-northeast-2",
    "ap-northeast-3",
    "ap-south-1",
    "ap-southeast-1",
    "ap-southeast-2",
    "ca-central-1",
    "eu-central-1",
    "eu-north-1",
    "eu-west-1",
    "eu-west-2",
    "eu-west-3",
    "sa-east-1",
    "us-east-1",
    "us-east-2",
    "us-west-1",
    "us-west-2",
  ]
  ami_groups = [
    # This causes the ami to be publicly-accessable.
    "all",
  ]
  ami_description = "Lotus Filecoin AMI"
  launch_block_device_mappings {
    device_name = "/dev/sda1"
    volume_size = 100
    delete_on_termination = true
  }

  instance_type = "t2.micro"
  source_ami_filter {
    filters = {
      name = "ubuntu/images/*ubuntu-focal-20.04-amd64-server-*"
      root-device-type = "ebs"
      virtualization-type = "hvm"
    }
    most_recent = true
    owners = ["099720109477"]
  }
  ssh_username = "ubuntu"
}

source "digitalocean" "lotus" {
  droplet_name = "lotus-${var.lotus_network}"
  size = "s-1vcpu-1gb"
  region = "nyc3"
  image = "ubuntu-20-04-x64"
  snapshot_name = "lotus-${var.lotus_network}-${var.git_tag}-${local.timestamp}"
  ssh_username = "root"
}

build {
  sources = [
    "source.amazon-ebs.lotus",
    "source.digitalocean.lotus",
  ]

  # Lotus software (from CI workspace)
  provisioner "file" {
    source = "${var.ci_workspace_bins}/lotus"
    destination = "lotus"
  }
  provisioner "file" {
    source = "${var.ci_workspace_bins}/lotus-miner"
    destination = "lotus-miner"
  }
  # First run script
  provisioner "file" {
    source = "./tools/packer/scripts/${var.lotus_network}/lotus-init.sh"
    destination = "lotus-init.sh"
  }
  # Systemd service units.
  provisioner "file" {
    source = "./tools/packer/systemd/lotus-daemon.service"
    destination = "lotus-daemon.service"
  }
  provisioner "file" {
    source = "./tools/packer/systemd/lotus-miner.service"
    destination = "lotus-miner.service"
  }
  provisioner "file" {
    source = "./tools/packer/repo/config.toml"
    destination = "config.toml"
  }
  provisioner "file" {
    source = "./tools/packer/etc/motd"
    destination = "motd"
  }
  provisioner "file" {
    source = "./tools/packer/homedir/bashrc"
    destination = ".bashrc"
  }
  # build it.
  provisioner "shell" {
    script = "./tools/packer/setup.sh"
  }
}
