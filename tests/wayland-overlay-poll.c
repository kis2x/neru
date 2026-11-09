// Run the production poll function against deterministic libwayland/syscall
// doubles. No display connection, input device, or keyboard injection is used.
#include <assert.h>
#include <errno.h>
#include <stdio.h>

#include "../internal/adapter/platform/linux/overlay_wayland.c"

static int prepared;
static int cancellations;
static int reads;
static int poll_error;
static int readable;

int wl_display_prepare_read(struct wl_display *display) {
	(void)display;
	assert(!prepared);
	prepared = 1;
	return 0;
}

int wl_display_dispatch_pending(struct wl_display *display) {
	(void)display;
	assert(!prepared);
	return 0;
}

int wl_display_flush(struct wl_display *display) {
	(void)display;
	return 0;
}

int wl_display_get_fd(struct wl_display *display) {
	(void)display;
	return 42;
}

void wl_display_cancel_read(struct wl_display *display) {
	(void)display;
	assert(prepared);
	prepared = 0;
	cancellations++;
}

int wl_display_read_events(struct wl_display *display) {
	(void)display;
	assert(prepared);
	prepared = 0;
	reads++;
	return 0;
}

int poll(struct pollfd *fds, nfds_t count, int timeout) {
	assert(prepared);
	assert(count == 1 && timeout == 0);
	assert(fds[0].fd == 42 && fds[0].events == POLLIN);
	if (poll_error) {
		errno = poll_error;
		return -1;
	}
	fds[0].revents = readable ? POLLIN : 0;
	return readable;
}

int main(void) {
	char display_token;
	NeruWaylandOverlay overlay = {.display = (struct wl_display *)&display_token};

	// A signal must release the prepared read without stopping the Go poller.
	poll_error = EINTR;
	assert(neru_wayland_overlay_poll(&overlay) == 0);
	assert(!prepared && cancellations == 1 && reads == 0);

	// The next iteration must still be able to read compositor events.
	poll_error = 0;
	readable = 1;
	assert(neru_wayland_overlay_poll(&overlay) == 1);
	assert(!prepared && cancellations == 1 && reads == 1);

	// An idle poll also balances prepare_read, and real errors remain fatal.
	readable = 0;
	assert(neru_wayland_overlay_poll(&overlay) == 0);
	assert(!prepared && cancellations == 2);
	poll_error = EIO;
	assert(neru_wayland_overlay_poll(&overlay) == -1);
	assert(!prepared && cancellations == 3);
	puts("Wayland poll: EINTR recovery, subsequent read, idle, and fatal error passed");
	return 0;
}
