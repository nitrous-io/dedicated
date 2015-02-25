#!/usr/bin/env bash
# this script setups the container for `dedicated`

echo "Generating SSH Key"

ssh-keygen -b 2048 -t rsa -f ~/.ssh/id_rsa -q -N ""
echo "--SSH KEY--"
cat ~/.ssh/id_rsa.pub
echo "--SSH KEY--"
echo "Press ENTER to continue"
read

DEDIREPO=$GOPATH/src/github.com/nitrous-io/dedicated
DEDILOCATION=$WORKSPACEDIR/dedicated

echo "Cloning repo into dedicated"
git clone git@github.com:nitrous-io/dedicated.git $DEDIREPO

echo "Setting up paths"
mkdir -p $GOPATH/src/github.com/nitrous-io
ln -nfs $DEDIREPO $DEDILOCATION
sudo su -c "cat >> /etc/profile << EOF
## DEV PATHS BEGIN
export GOPATH=$DEDIREPO/Godeps/_workspace:$GOPATH
export GOROOT=$GOROOT
export PATH=$DEDIREPO/Godeps/_workspace/bin:$GOPATH/bin:$GOROOT/bin:$RUBYBIN:\\$PATH
export DEDILOCATION=$DEDILOCATION
## DEV PATHS END
EOF"

echo "Configuring dedicated"
sudo /etc/init.d/postgresql start
sudo /etc/init.d/nginx start
cd $DEDILOCATION
cp nitrous/.env-example nitrous/.env
sed -i 's/${PWD}\/dev/${PWD}\/nitrous\/dev/' nitrous/.env
make createdb
make nitrous
sed -i 's/user=vagrant/user=nitrous/' nitrous/.env
PWD=/nitrous forego run -e nitrous/.env build/dbseed