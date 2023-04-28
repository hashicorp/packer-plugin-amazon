# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

echo "installing nginx "
sudo apt-get update
sudo apt-get install nginx -y
sudo service nginx restart
