#include <algorithm>
#include <array>
#include <condition_variable>
#include <cstdlib>
#include <cstring>
#include <filesystem>
#include <iostream>
#include <mutex>
#include <optional>
#include <queue>
#include <regex>
#include <string>
#include <thread>
#include <unordered_set>
#include <utility>
#include <vector>

#include <spawn.h>
#include <sys/wait.h>
#include <unistd.h>

extern char **environ;

struct DownloadResult {
    std::string url;
    bool ok;
    std::string msg;
};

struct CommandResult {
    int exitCode{ -1 };
    bool spawnError{ false };
    bool notFound{ false };
    std::string output;
};

struct PromptResult {
    std::vector<std::string> rawUrls;
    bool shouldQuit{ false };
};

struct UrlParts {
    std::string scheme;
    std::string host;
    std::string path;
    std::string query;
};

const std::regex kUrlToken(R"((https?://\S+|video\.twimg\.com/\S+))");
const std::string kTrimChars = "><()[]{}.,;:\"'`";

bool startsWith(const std::string &value, const std::string &prefix) {
    return value.rfind(prefix, 0) == 0;
}

std::string trimWhitespace(const std::string &input) {
    auto begin = std::find_if_not(input.begin(), input.end(), [](unsigned char ch) { return std::isspace(ch); });
    auto end = std::find_if_not(input.rbegin(), input.rend(), [](unsigned char ch) { return std::isspace(ch); }).base();
    if (begin >= end) {
        return "";
    }
    return std::string(begin, end);
}

std::string trimCharacters(const std::string &input, const std::string &chars) {
    auto start = input.find_first_not_of(chars);
    if (start == std::string::npos) {
        return "";
    }
    auto end = input.find_last_not_of(chars);
    return input.substr(start, end - start + 1);
}

int defaultWorkers() {
    const unsigned int cpus = std::thread::hardware_concurrency();
    if (cpus == 0 || cpus < 2) {
        return 1;
    }
    return static_cast<int>(cpus / 2);
}

int clampWorkers(int requested, std::size_t urlCount) {
    if (requested < 1) {
        return 1;
    }
    if (static_cast<std::size_t>(requested) > urlCount) {
        return static_cast<int>(urlCount);
    }
    return requested;
}

bool expandPath(const std::string &input, std::string &expanded) {
    if (input.empty()) {
        return false;
    }

    if (startsWith(input, "~")) {
        const char *homeEnv = std::getenv("HOME");
        if (!homeEnv) {
            homeEnv = std::getenv("USERPROFILE");
        }
        if (!homeEnv) {
            return false;
        }
        std::filesystem::path home(homeEnv);
        std::string remainder = input.substr(1);
        while (!remainder.empty() && (remainder.front() == '/' || remainder.front() == '\\')) {
            remainder.erase(remainder.begin());
        }
        expanded = (home / remainder).string();
    } else {
        expanded = input;
    }

    expanded = std::filesystem::path(expanded).lexically_normal().string();
    return true;
}

void printUsage(const std::string &program) {
    std::cout << "Usage: " << program << " [-dir <path>] [-workers <num>]\n";
}

PromptResult promptURLs() {
    std::cout << "Paste MP4 URLs (one per line). Blank lines are ignored. Type ':go' to start, ':q' to quit.\n";

    PromptResult result;
    for (;;) {
        std::cout << "> " << std::flush;
        std::string line;
        if (!std::getline(std::cin, line)) {
            const std::string stripped = trimWhitespace(line);
            if (!stripped.empty()) {
                result.rawUrls.push_back(line);
            }
            result.shouldQuit = true;
            return result;
        }

        const std::string stripped = trimWhitespace(line);
        if (stripped == ":q" || stripped == ":quit" || stripped == ":exit") {
            result.shouldQuit = true;
            return result;
        }
        if (stripped == ":go" || stripped == ":start" || stripped == ":run") {
            return result;
        }
        if (stripped.empty()) {
            continue;
        }
        result.rawUrls.push_back(line);
    }
}

bool parseUrlBasic(const std::string &url, UrlParts &parts) {
    const auto schemePos = url.find("://");
    if (schemePos == std::string::npos) {
        return false;
    }
    parts.scheme = url.substr(0, schemePos);

    std::string withoutFragment = url;
    const auto fragPos = withoutFragment.find('#');
    if (fragPos != std::string::npos) {
        withoutFragment = withoutFragment.substr(0, fragPos);
    }

    const std::size_t hostStart = schemePos + 3;
    if (hostStart >= withoutFragment.size()) {
        return false;
    }

    const auto pathStart = withoutFragment.find_first_of("/?", hostStart);
    if (pathStart == std::string::npos) {
        parts.host = withoutFragment.substr(hostStart);
        parts.path.clear();
        parts.query.clear();
        return !parts.host.empty();
    }

    parts.host = withoutFragment.substr(hostStart, pathStart - hostStart);
    if (parts.host.empty()) {
        return false;
    }

    const auto queryPos = withoutFragment.find('?', pathStart);
    if (queryPos == std::string::npos) {
        parts.path = withoutFragment.substr(pathStart);
        parts.query.clear();
        return true;
    }

    parts.path = withoutFragment.substr(pathStart, queryPos - pathStart);
    parts.query = withoutFragment.substr(queryPos + 1);
    return true;
}

std::string filterQuery(const std::string &query) {
    if (query.empty()) {
        return "";
    }
    std::vector<std::string> kept;
    std::size_t pos = 0;
    while (pos < query.size()) {
        const auto next = query.find('&', pos);
        std::string part = (next == std::string::npos) ? query.substr(pos) : query.substr(pos, next - pos);
        pos = (next == std::string::npos) ? query.size() : next + 1;

        if (part.empty()) {
            continue;
        }
        const auto eq = part.find('=');
        const std::string key = part.substr(0, eq);
        if (key == "tag") {
            continue;
        }
        kept.push_back(part);
    }

    std::string rebuilt;
    for (std::size_t i = 0; i < kept.size(); ++i) {
        if (i > 0) {
            rebuilt.push_back('&');
        }
        rebuilt += kept[i];
    }
    return rebuilt;
}

std::optional<std::string> cleanURL(const std::string &raw) {
    const std::string text = trimWhitespace(raw);
    if (text.empty()) {
        return std::nullopt;
    }

    std::smatch match;
    if (!std::regex_search(text, match, kUrlToken)) {
        return std::nullopt;
    }

    std::string candidate = trimCharacters(match.str(), kTrimChars);
    if (!startsWith(candidate, "http://") && !startsWith(candidate, "https://")) {
        std::string stripped = candidate;
        while (!stripped.empty() && stripped.front() == '/') {
            stripped.erase(stripped.begin());
        }
        candidate = "https://" + stripped;
    }

    UrlParts parts;
    if (!parseUrlBasic(candidate, parts)) {
        return std::nullopt;
    }

    parts.query = filterQuery(parts.query);

    std::string normalized = parts.scheme + "://" + parts.host;
    normalized += parts.path;
    if (!parts.query.empty()) {
        normalized.push_back('?');
        normalized += parts.query;
    }
    return normalized;
}

std::vector<std::string> gatherURLs(const std::vector<std::string> &raw) {
    std::unordered_set<std::string> seen;
    std::vector<std::string> cleaned;
    cleaned.reserve(raw.size());

    for (const auto &candidate : raw) {
        auto url = cleanURL(candidate);
        if (url && seen.insert(*url).second) {
            cleaned.push_back(*url);
        }
    }
    return cleaned;
}

CommandResult runCommand(const std::vector<std::string> &args) {
    CommandResult result;
    if (args.empty()) {
        result.spawnError = true;
        result.output = "no command specified";
        return result;
    }

    int pipefd[2];
    if (pipe(pipefd) != 0) {
        result.spawnError = true;
        result.output = std::strerror(errno);
        return result;
    }

    posix_spawn_file_actions_t actions;
    posix_spawn_file_actions_init(&actions);
    posix_spawn_file_actions_adddup2(&actions, pipefd[1], STDOUT_FILENO);
    posix_spawn_file_actions_adddup2(&actions, pipefd[1], STDERR_FILENO);
    posix_spawn_file_actions_addclose(&actions, pipefd[0]);

    std::vector<char *> argv;
    argv.reserve(args.size() + 1);
    for (const auto &arg : args) {
        argv.push_back(strdup(arg.c_str()));
    }
    argv.push_back(nullptr);

    pid_t pid = -1;
    const int spawnStatus = posix_spawnp(&pid, args[0].c_str(), &actions, nullptr, argv.data(), environ);
    posix_spawn_file_actions_destroy(&actions);
    close(pipefd[1]);

    if (spawnStatus != 0) {
        close(pipefd[0]);
        result.spawnError = true;
        result.notFound = (spawnStatus == ENOENT);
        result.output = std::strerror(spawnStatus);
        for (char *ptr : argv) {
            free(ptr);
        }
        return result;
    }

    std::array<char, 4096> buffer{};
    ssize_t n = 0;
    while ((n = read(pipefd[0], buffer.data(), buffer.size())) > 0) {
        result.output.append(buffer.data(), static_cast<std::size_t>(n));
    }
    close(pipefd[0]);

    int status = 0;
    waitpid(pid, &status, 0);
    if (WIFEXITED(status)) {
        result.exitCode = WEXITSTATUS(status);
    } else {
        result.exitCode = -1;
    }

    for (char *ptr : argv) {
        free(ptr);
    }
    return result;
}

DownloadResult downloadOne(const std::string &targetURL, const std::string &destDir) {
    const std::vector<std::string> args = { "wget", "-c", "-P", destDir, targetURL };
    CommandResult cmd = runCommand(args);

    if (!cmd.spawnError && cmd.exitCode == 0) {
        return { targetURL, true, "ok" };
    }
    if (cmd.notFound) {
        return { targetURL, false, "wget not found; install wget and retry" };
    }

    std::string msg = trimWhitespace(cmd.output);
    if (msg.empty()) {
        msg = cmd.spawnError ? "failed to launch wget" : "wget failed";
    }
    return { targetURL, false, msg };
}

std::vector<DownloadResult> downloadAll(const std::vector<std::string> &urls, const std::string &destDir, int workers) {
    if (workers <= 1 || urls.size() <= 1) {
        std::vector<DownloadResult> results;
        results.reserve(urls.size());
        for (const auto &u : urls) {
            results.push_back(downloadOne(u, destDir));
        }
        return results;
    }

    std::queue<std::string> jobs;
    for (const auto &u : urls) {
        jobs.push(u);
    }

    std::vector<DownloadResult> results;
    results.reserve(urls.size());
    std::mutex jobMutex;
    std::mutex resultMutex;

    auto worker = [&]() {
        for (;;) {
            std::string url;
            {
                std::lock_guard<std::mutex> lock(jobMutex);
                if (jobs.empty()) {
                    return;
                }
                url = std::move(jobs.front());
                jobs.pop();
            }
            DownloadResult res = downloadOne(url, destDir);
            {
                std::lock_guard<std::mutex> lock(resultMutex);
                results.push_back(std::move(res));
            }
        }
    };

    std::vector<std::thread> threads;
    threads.reserve(static_cast<std::size_t>(workers));
    for (int i = 0; i < workers; ++i) {
        threads.emplace_back(worker);
    }
    for (auto &t : threads) {
        t.join();
    }
    return results;
}

void report(const std::vector<DownloadResult> &results) {
    std::vector<std::string> success;
    std::vector<DownloadResult> failed;
    for (const auto &res : results) {
        if (res.ok) {
            success.push_back(res.url);
        } else {
            failed.push_back(res);
        }
    }

    if (!success.empty()) {
        std::cout << "Downloaded " << success.size() << " file(s).\n";
    }
    if (!failed.empty()) {
        std::cout << "Failed " << failed.size() << " file(s):\n";
        for (const auto &res : failed) {
            std::cout << "- " << res.url << " :: " << res.msg << "\n";
        }
    }
}

int main(int argc, char *argv[]) {
    std::string destFlag = "~/Downloads/mobile/";
    int workersFlag = defaultWorkers();

    for (int i = 1; i < argc; ++i) {
        std::string arg(argv[i]);
        if (arg == "-dir" || arg == "--dir") {
            if (i + 1 >= argc) {
                std::cerr << "Missing value for " << arg << ".\n";
                printUsage(argv[0]);
                return 1;
            }
            destFlag = argv[++i];
        } else if (arg == "-workers" || arg == "--workers") {
            if (i + 1 >= argc) {
                std::cerr << "Missing value for " << arg << ".\n";
                printUsage(argv[0]);
                return 1;
            }
            workersFlag = std::max(1, std::stoi(argv[++i]));
        } else if (arg == "-h" || arg == "--help") {
            printUsage(argv[0]);
            return 0;
        } else {
            std::cerr << "Unknown argument: " << arg << "\n";
            printUsage(argv[0]);
            return 1;
        }
    }

    std::string destDir;
    if (!expandPath(destFlag, destDir)) {
        std::cerr << "resolve download directory: could not expand path\n";
        return 1;
    }

    std::error_code ec;
    std::filesystem::create_directories(destDir, ec);
    if (ec) {
        std::cerr << "create download directory: " << ec.message() << "\n";
        return 1;
    }

    for (;;) {
        PromptResult prompt = promptURLs();
        std::vector<std::string> urls = gatherURLs(prompt.rawUrls);

        if (prompt.shouldQuit && urls.empty()) {
            std::cout << "Goodbye.\n";
            return 0;
        }

        if (urls.empty()) {
            std::cout << "No URLs provided. Paste URLs or type :q to quit.\n";
            if (prompt.shouldQuit) {
                return 0;
            }
            continue;
        }

        const int workerCount = clampWorkers(workersFlag, urls.size());
        std::cout << "Downloading " << urls.size() << " file(s) to " << destDir << " with " << workerCount << " worker(s)...\n";

        std::vector<DownloadResult> results = downloadAll(urls, destDir, workerCount);
        report(results);

        std::cout << "Batch complete.\n\n";
        if (prompt.shouldQuit) {
            return 0;
        }
    }
}
