#!/bin/sh -eux
# Copyright 2018 Mikhail Klementev. All rights reserved.
# Use of this source code is governed by a AGPLv3 license
# (or later) that can be found in the LICENSE file.
TMPDIR=$1
chroot $TMPDIR /bin/sh -c 'useradd -m user'
sed -i 's/root:\*:/root::/' $TMPDIR/etc/shadow
sed -i 's/user:!!:/user::/' $TMPDIR/etc/shadow
echo auth sufficient pam_permit.so > $TMPDIR/etc/pam.d/sshd
sed -i '/PermitEmptyPasswords/d' $TMPDIR/etc/ssh/sshd_config
echo PermitEmptyPasswords yes >> $TMPDIR/etc/ssh/sshd_config
sed -i '/PermitRootLogin/d' $TMPDIR/etc/ssh/sshd_config
echo PermitRootLogin yes >> $TMPDIR/etc/ssh/sshd_config

echo '#!/bin/sh' > $TMPDIR/etc/rc.local
echo 'dhclient' >> $TMPDIR/etc/rc.local
chmod +x $TMPDIR/etc/rc.local
