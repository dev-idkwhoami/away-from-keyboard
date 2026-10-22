// AFK's Linux terminal view. Keep inference independent of presentation.
#pragma once
#include <algorithm>
#include <clocale>
#include <cstdio>
#include <cstring>
#include <cstdlib>
#include <cwchar>
#include <cwctype>
#include <string>
#include <sys/ioctl.h>
#include <unistd.h>
#include <vector>

namespace afk_terminal {
struct character { std::string bytes; int cells; };

// Normalize whitespace/control characters and retain the newest complete UTF-8
// characters that fit. Width is measured in terminal cells, not bytes.
inline std::string tail(const std::string & input, int columns) {
    if (columns <= 0) return {};
    std::vector<character> chars;
    std::mbstate_t state{};
    bool space = true;
    for (size_t i = 0; i < input.size();) {
        wchar_t wc;
        size_t n = std::mbrtowc(&wc, input.data() + i, input.size() - i, &state);
        if (n == size_t(-1) || n == size_t(-2)) {
            state = {}; ++i; continue;
        }
        if (n == 0) { ++i; continue; }
        std::string bytes = input.substr(i, n); i += n;
        int cells = ::wcwidth(wc);
        if (std::iswspace(wc) || std::iswcntrl(wc) || cells < 0) {
            if (!space) chars.push_back({" ", 1});
            space = true; continue;
        }
        chars.push_back({bytes, cells}); space = false;
    }
    if (!chars.empty() && chars.back().bytes == " ") chars.pop_back();
    int used = 0;
    size_t begin = chars.size();
    while (begin > 0 && used + chars[begin - 1].cells <= columns) {
        used += chars[--begin].cells;
    }
    while (begin < chars.size() && (chars[begin].cells == 0 || chars[begin].bytes == " ")) ++begin;
    std::string result;
    for (size_t i = begin; i < chars.size(); ++i) result += chars[i].bytes;
    return result;
}

inline int width() {
    winsize size{};
    return ::ioctl(STDOUT_FILENO, TIOCGWINSZ, &size) == 0 && size.ws_col > 0 ? size.ws_col : 80;
}

class line {
    std::string committed, pending;
    bool events = std::getenv("AFK_EVENTS") != nullptr;
    void emit(const char * type, const std::string & text) {
        std::printf("{\"type\":\"%s\",\"text\":\"", type);
        for (unsigned char c : text) {
            if (c == '"' || c == '\\') std::printf("\\%c", c);
            else if (c < 32) std::printf("\\u%04x", c);
            else std::putchar(c);
        }
        std::printf("\"}\n"); std::fflush(stdout);
    }
public:
    line() { std::setlocale(LC_CTYPE, ""); }
    void update(const std::string & text) {
        pending = tail(text, 4096);
        if (events) { emit("partial", pending); return; }
        const auto visible = tail(committed + " " + pending, std::max(0, width() - 1));
        // Erase in place; never print a padding line or fill the last column.
        std::printf("\r\033[2K%s", visible.c_str());
        std::fflush(stdout);
    }
    void commit() {
        if (events) emit("commit", pending);
        committed = tail(committed + " " + pending, 4096);
        pending.clear();
    }
};
} // namespace afk_terminal
