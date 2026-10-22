#include "terminal.h"
#include <cassert>
#include <pty.h>
#include <fcntl.h>

int main(int argc, char **) {
    if (argc > 1) {
        setenv("AFK_EVENTS", "1", 1);
        afk_terminal::line events;
        events.update("Grüße \"AFK\" \\ test");
        events.commit();
        events.update("next");
        return 0;
    }
    unsetenv("AFK_EVENTS");
    std::setlocale(LC_CTYPE, "C.UTF-8");
    using afk_terminal::tail;
    assert(tail("old words newest", 6) == "newest");
    assert(tail("Grüße für Köln", 4) == "Köln");
    assert(tail("old 世界", 4) == "世界");
    assert(tail("old 世界", 3) == "界");
    assert(tail("old e\xcc\x81", 1) == "e\xcc\x81");
    assert(tail("hello\n\tworld\r", 80) == "hello world");
    assert(tail("anything", 0).empty());

    // Exercise the real renderer against a narrow terminal, including resize
    // and an audio-window commit. No update may contain a newline or wrap.
    int master, slave;
    winsize size{}; size.ws_row = 24; size.ws_col = 12;
    assert(openpty(&master, &slave, nullptr, nullptr, &size) == 0);
    const int saved = dup(STDOUT_FILENO);
    assert(dup2(slave, STDOUT_FILENO) >= 0);
    afk_terminal::line line;
    line.update("a very long sentence with newest");
    char output[1024];
    auto n = read(master, output, sizeof(output));
    assert(std::string(output, n) == "\r\033[2Kwith newest");
    line.commit();
    size.ws_col = 8; assert(ioctl(slave, TIOCSWINSZ, &size) == 0);
    line.update("Grüße");
    n = read(master, output, sizeof(output));
    assert(std::string(output, n) == "\r\033[2Kt Grüße");
    assert(dup2(saved, STDOUT_FILENO) >= 0);
    close(saved); close(slave); close(master);
}
