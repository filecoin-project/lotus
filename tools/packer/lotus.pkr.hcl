locals { timestamp = regex_replace(timestamp(), "[- TZ:]", "") }

source "amazon-ebs" "lotus-mainnet" {
  ami_name      = "lotus-mainnet-${local.timestamp}"
  ami_regions = [
    "us-east-1",
    "us-west-2",
  ]
  ami_description = "Lotus Filecoin AMI"

  instance_type = "t2.micro"
  source_ami_filter {
    filters = {
      name = "ubuntu-minimal/images/*ubuntu-focal-20.04-amd64-minimal*"
      root-device-type = "ebs"
      virtualization-type = "hvm"
    }
    most_recent = true
    owners = ["099720109477"]
  }
  ssh_username = "ubuntu"
}

source "digitalocean" "lotus-mainnet" {
  droplet_name = "lotus-mainnet"
  size = "s-1vcpu-1gb"
  region = "nyc3"
  image = "ubuntu-20-04-x64"
  snapshot_name = "lotus-mainnet-${local.timestamp}"
  ssh_username = "root"
}

build {
  sources = [
    "source.amazon-ebs.lotus-mainnet",
    "source.digitalocean.lotus-mainnet",
  ]

  # Lotus software (from CI workspace)
  provisioner "file" {
    source = "./linux/lotus"
    destination = "lotus"
  }
  provisioner "file" {
    source = "./linux/lotus-miner"
    destination = "lotus-miner"
  }
  # First run script
  provisioner "file" {
    source = "./tools/packer/scripts/lotus-init.sh"
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
    source = "./tools/packer/systemd/lotus-init.service"
    destination = "lotus-init.service"
  }
  provisioner "file" {
    source = "./tools/packer/LOTUS.txt"
    destination = "LOTUS.txt"
  }
  provisioner "file" {
    source = "./tools/packer/bashrc"
    destination = ".bashrc"
  }
  # build it.
  provisioner "shell" {
    script = "./tools/packer/setup.sh"
  }

}
