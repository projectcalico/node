#!/bin/bash
if [[ "$BUILDARCH" == "aarch64" ]]; then
# In order to rebuild the iptables RPM, we first need to rebuild the libnftnl RPM because building
# iptables requires libnftnl-devel but libnftnl-devel is not available on ubi or CentOS repos.
# (Note: it's not in RHEL8.1 either https://bugzilla.redhat.com/show_bug.cgi?id=1711361).
# Rebuilding libnftnl will give us libnftnl-devel too.
rpm -i ${LIBNFTNL_SOURCERPM_URL} && \
    yum-builddep -y --spec /root/rpmbuild/SPECS/libnftnl.spec && \
    rpmbuild -bb /root/rpmbuild/SPECS/libnftnl.spec && \
# Now install libnftnl and libnftnl-devel
rpm -Uv /root/rpmbuild/RPMS/${ARCH}/libnftnl-${LIBNFTNL_VER}.el8.${ARCH}.rpm && \
rpm -Uv /root/rpmbuild/RPMS/${ARCH}/libnftnl-devel-${LIBNFTNL_VER}.el8.${ARCH}.rpm && \
# Install source RPM for iptables and install its build dependencies.
rpm -i ${IPTABLES_SOURCERPM_URL} && \
yum-builddep -y --spec /root/rpmbuild/SPECS/iptables.spec

# Patch the iptables build spec so that we keep the legacy iptables binaries.
sed -i '/drop all legacy tools/,/sbindir.*legacy/d' /root/rpmbuild/SPECS/iptables.spec

# Patch the iptables build spec to drop the renaming of nft binaries. Instead of renaming binaries,
# we will use alternatives to set the canonical iptables binaries.
sed -i '/rename nft versions to standard name/,/^done/d' /root/rpmbuild/SPECS/iptables.spec

# Patch the iptables build spec so that legacy and nft iptables binaries are verified to be in the resulting rpm.
sed -i '/%files$/a \
\%\{_sbindir\}\/xtables-legacy-multi \n\
\%\{_sbindir\}\/ip6tables-legacy \n\
\%\{_sbindir\}\/ip6tables-legacy-restore \n\
\%\{_sbindir\}\/ip6tables-legacy-save \n\
\%\{_sbindir\}\/iptables-legacy \n\
\%\{_sbindir\}\/iptables-legacy-restore \n\
\%\{_sbindir\}\/iptables-legacy-save \n\
\%\{_sbindir\}\/ip6tables-nft\n\
\%\{_sbindir\}\/ip6tables-nft-restore\n\
\%\{_sbindir\}\/ip6tables-nft-save\n\
\%\{_sbindir\}\/iptables-nft\n\
\%\{_sbindir\}\/iptables-nft-restore\n\
\%\{_sbindir\}\/iptables-nft-save\n\
' /root/rpmbuild/SPECS/iptables.spec

# Finally rebuild iptables.
rpmbuild -bb /root/rpmbuild/SPECS/iptables.spec

else

mkdir -p /root/rpmbuild/RPMS/${ARCH}/ 
# Iptables requirements
cp /root/*.rpm /root/rpmbuild/RPMS/${ARCH}/ 
fi