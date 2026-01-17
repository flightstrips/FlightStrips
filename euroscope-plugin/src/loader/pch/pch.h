// pch.h: This is a precompiled header file.
// Files listed below are compiled only once, improving build performance for future builds.
// This also affects IntelliSense performance, including code completion and many code browsing features.
// However, files listed here are ALL re-compiled if any one of them is updated between builds.
// Do not add files here that you will be updating frequently as this negates the performance advantage.

#pragma once

#ifndef ISOLATION_AWARE_ENABLED
#define ISOLATION_AWARE_ENABLED 1 // NOLINT
#endif
#define _WIN32_WINNT 0x0603
#define _SILENCE_CXX20_UNCAUGHT_EXCEPTION_DEPRECATION_WARNING // NOLINT
#define _SILENCE_CXX20_CODECVT_HEADER_DEPRECATION_WARNING     // NOLINT
#define _SILENCE_CXX20_ALLOCATOR_VOID_DEPRECATION_WARNING     // NOLINT
#define NOMINMAX 1

// Custom headers
#pragma warning(push)
#pragma warning(disable : 26495 26451)

// Standard headers
#include <ws2tcpip.h>
#include <winsock2.h>
#include <Windows.h>
#include <CommCtrl.h>
#include <CommDlg.h>
#include <Shlobj.h>
#include <Shobjidl.h>
#include <algorithm>
#include <cctype>
#include <codecvt>
#include <ctime>
#include <fstream>
#include <iterator>
#include <locale>
#include <map>
#include <mmsystem.h>
#include <mutex>
#include <queue>
#include <regex>
#include <set>
#include <shellapi.h>
#include <sstream>
#include <string>
#include <tchar.h>
#include <type_traits>
#include <typeindex>
#include <unordered_set>
#include <utility>
#include <vector>
#include <array>
#include <gdiplus.h>

#include <euroscope/EuroScopePlugIn.h>