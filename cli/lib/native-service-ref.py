"""Hold one systemd unit reference on one private sd-bus connection.

Only the fixed line protocol is public. Native errors and properties never leave
this process. EOF, invalid input and process exit all release the reference.
"""

import ctypes
import os
import select
import sys


UNIT = b"my-api.service"
DESTINATION = b"org.freedesktop.systemd1"
MANAGER_PATH = b"/org/freedesktop/systemd1"
MANAGER_INTERFACE = b"org.freedesktop.systemd1.Manager"


def run():
    if len(sys.argv) != 2 or sys.argv[1] not in ("system", "user"):
        raise RuntimeError()
    scope = sys.argv[1]
    if os.getuid() != os.geteuid() or (os.getuid() == 0) != (scope == "system"):
        raise RuntimeError()

    library = ctypes.CDLL("libsystemd.so.0")
    libc = ctypes.CDLL(None)
    pointer = ctypes.c_void_p
    string = ctypes.c_char_p
    library.sd_bus_open_system.argtypes = [ctypes.POINTER(pointer)]
    library.sd_bus_open_user.argtypes = [ctypes.POINTER(pointer)]
    library.sd_bus_set_method_call_timeout.argtypes = [pointer, ctypes.c_uint64]
    library.sd_bus_get_unique_name.argtypes = [pointer, ctypes.POINTER(string)]
    library.sd_bus_call_method.argtypes = [pointer, string, string, string, string,
                                          pointer, ctypes.POINTER(pointer), string]
    library.sd_bus_message_read.argtypes = [pointer, string]
    library.sd_bus_get_property_strv.argtypes = [pointer, string, string, string, string,
                                                pointer, ctypes.POINTER(ctypes.POINTER(pointer))]
    library.sd_bus_message_unref.argtypes = [pointer]
    library.sd_bus_message_unref.restype = pointer
    library.sd_bus_close_unref.argtypes = [pointer]
    library.sd_bus_close_unref.restype = pointer
    libc.free.argtypes = [pointer]
    libc.free.restype = None

    bus = pointer()
    held = False

    def checked(result):
        if result < 0:
            raise RuntimeError()

    def call(member, reply=None):
        checked(library.sd_bus_call_method(bus, DESTINATION, MANAGER_PATH,
                                          MANAGER_INTERFACE, member, None, reply, b"s", string(UNIT)))

    def assert_reference():
        unique = string()
        checked(library.sd_bus_get_unique_name(bus, ctypes.byref(unique)))
        reply = pointer()
        try:
            call(b"GetUnit", ctypes.byref(reply))
            unit_path = string()
            if library.sd_bus_message_read(reply, b"o", ctypes.byref(unit_path)) <= 0:
                raise RuntimeError()
            refs = ctypes.POINTER(pointer)()
            try:
                checked(library.sd_bus_get_property_strv(bus, DESTINATION, unit_path,
                                                        b"org.freedesktop.systemd1.Unit", b"Refs",
                                                        None, ctypes.byref(refs)))
                found = False
                index = 0
                while refs[index]:
                    if index > 4096:
                        raise RuntimeError()
                    if ctypes.cast(refs[index], string).value == unique.value:
                        found = True
                    index += 1
                if not found:
                    raise RuntimeError()
            finally:
                if refs:
                    index = 0
                    while refs[index]:
                        libc.free(refs[index])
                        index += 1
                    libc.free(refs)
        finally:
            if reply:
                library.sd_bus_message_unref(reply)

    try:
        opener = library.sd_bus_open_system if scope == "system" else library.sd_bus_open_user
        checked(opener(ctypes.byref(bus)))
        checked(library.sd_bus_set_method_call_timeout(bus, 5_000_000))
        call(b"RefUnit")
        held = True
        assert_reference()
        print("held", flush=True)
        pending = b""
        while True:
            # A small bounded protocol; never accept a unit name, address or D-Bus method.
            if not select.select([sys.stdin.buffer], [], [], 5)[0]:
                assert_reference()
                continue
            chunk = os.read(sys.stdin.fileno(), 32)
            if not chunk:
                break
            pending += chunk
            if len(pending) > 64:
                raise RuntimeError()
            while b"\n" in pending:
                command, pending = pending.split(b"\n", 1)
                if command == b"check":
                    assert_reference()
                    print("held", flush=True)
                elif command == b"release" and not pending:
                    call(b"UnrefUnit")
                    held = False
                    print("released", flush=True)
                    return
                else:
                    raise RuntimeError()
        if pending:
            raise RuntimeError()
    finally:
        try:
            if held:
                call(b"UnrefUnit")
        finally:
            if bus:
                library.sd_bus_close_unref(bus)


if __name__ == "__main__":
    try:
        run()
    except BaseException:
        # No traceback, native error message, address, or log content is exposed.
        sys.stderr.write("MyAPI service reference unavailable.\n")
        sys.exit(2)
