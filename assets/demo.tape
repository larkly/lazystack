# lazystack demo tape — showcases the TUI against a live Talos/OpenStack cluster.
#
# Usage:
#   export OS_CLOUD=openstack
#   vhs assets/demo.tape
#
# Produces assets/lazystack-demo.gif
#
# Notes:
#   - The Images step performs a REAL upload of /tmp/openstack-amd64.raw
#     into Glance as `talos-openstack-1.12.6`. Make sure the file exists and
#     that the target image name is not already taken.
#   - Tune the Sleep after Ctrl+S to match your upload bandwidth (4.2 GB).

Output assets/lazystack-demo.gif

Set Shell "bash"
Set FontSize 18
Set Width 1500
Set Height 900
Set Padding 18
Set TypingSpeed 45ms
Set PlaybackSpeed 1.0
Set Theme "Builtin Solarized Dark"

# --- Prep (hidden) ---
Hide
Type "export OS_CLOUD=openstack"
Enter
Type "clear"
Enter
Show

Sleep 800ms

# --- Launch ---
Hide
Type "src/lazystack --no-check-update --refresh 1"
Enter
Show

# Wait for Keystone + first Nova page.
Sleep 5s

# --- Server list
Down
Sleep 500ms
Down
Sleep 500ms
Down
Sleep 500ms
Down
Sleep 250ms
Enter
Sleep 2500ms
Escape
Sleep 1s

# --- Create a server ---
Ctrl+N
Sleep 1s
Type "testserver"
Tab
Sleep 500ms
Enter
Type "deb"
Sleep 500ms
Enter
Sleep 500ms
Enter
Sleep 500ms
Type ".2ram"
Sleep 500ms
Enter
Sleep 500ms
Enter
Sleep 500ms
Enter
Sleep 500ms
Enter
Sleep 500ms
Type "test"
Sleep 500ms
Enter
Sleep 500ms
Enter
Sleep 500ms
Type "ssh"
Sleep 500ms
Enter
Ctrl+S
Sleep 5s
Type "/"
Sleep 500ms
Type "tests"
Sleep 500ms
Ctrl+D
Sleep 500ms
Left
Sleep 500ms
Enter
Sleep 500ms
Escape
Escape

# --- Volumes tab ---
Type "2"
Sleep 3s
Down
Sleep 500ms
Down
Sleep 500ms
Enter
Sleep 2500ms
Escape
Sleep 800ms

# --- Images tab: upload a Talos image ---
Type "3"
Sleep 2500ms

# Open upload form
Ctrl+N
Sleep 1500ms

# Name
Tab
Type "aaa-test"
Sleep 600ms
Tab

# Path to local file
Type "/tmp/cilium.tar.gz"
Sleep 800ms
Tab

# Disk format: qcow2 (default) -> raw
Right
Sleep 1500ms

# Submit upload
Ctrl+S

# Show the upload progress in the status bar for a while.
# 4.2 GB on a 1 Gbit link is ~35 s; scale this up if your link is slower.
Sleep 3s

# --- Floating IPs ---
Type "4"
Sleep 3s

# --- Security groups: expand rules on the highlighted group ---
Type "5"
Down
Sleep 2s
Tab 2
Ctrl+N
Sleep 2s
Escape
Sleep 500ms
Escape
Sleep 800ms

# --- Networks ---
Type "6"
Sleep 2s
Tab 2
Sleep 500ms
Ctrl+N
Sleep 2s
Escape
Sleep 800ms

# --- Routers: open one, see interfaces/routes ---
Type "7"
Sleep 2s
Ctrl+N
Sleep 2s
Escape
Sleep 800ms

# --- Load balancers: Octavia tree view ---
Type "8"
Sleep 2s
Tab 3
Sleep 500ms
Ctrl+N
Sleep 2s
Escape
Sleep 800ms

# --- Key pairs ---
Type "9"
Sleep 2500ms
Ctrl+N
Type "testkey"
Enter
Sleep 500ms
Right Right
Sleep 500ms
Ctrl+S
Sleep 3s
Escape
Sleep 1500ms

# --- Quota overlay ---
Type "Q"
Sleep 4s
Escape
Sleep 600ms

# --- Help overlay ---
Type "?"
Sleep 3500ms
Type "?"
Sleep 2s
Escape
Sleep 600ms

# --- Quit ---
Type "q"
Sleep 1500ms
