cat <<EOF > "$1"
image: $2:$3
manifests:
  - image: $2:$3-windows-1809
    platform:
      architecture: amd64
      os: windows
  - image: $2:$3-windows-2004
    platform:
      architecture: amd64
      os: windows
  - image: $2:$3-windows-20H2
    platform:
      architecture: amd64
      os: windows
  - image: $2:$3-windows-ltsc2022
    platform:
      architecture: amd64
      os: windows
EOF
