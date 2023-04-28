# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

echo "installing apache "
sudo apt-get update
sudo apt-get install apache2 -y
sudo apt-get update
sudo service apache2 restart
sudo apache2 --version
