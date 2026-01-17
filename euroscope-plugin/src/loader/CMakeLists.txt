set(PROJECT_NAME FlightStripsLoader)

################################################################################
# Source groups
################################################################################
set(ALL_FILES dllmain.cpp
        Loader.cpp
        Loader.h)

################################################################################
# Target
################################################################################
add_library(${PROJECT_NAME} SHARED ${ALL_FILES})

target_precompile_headers(${PROJECT_NAME} PRIVATE "$<$<COMPILE_LANGUAGE:CXX>:${CMAKE_CURRENT_SOURCE_DIR}/pch/pch.h>")

use_props(${PROJECT_NAME} "${CMAKE_CONFIGURATION_TYPES}" "${DEFAULT_CXX_PROPS}")
set(ROOT_NAMESPACE FlightStrips)

set_target_properties(${PROJECT_NAME} PROPERTIES
        VS_GLOBAL_KEYWORD "Win32Proj"
)

################################################################################
# Target name
################################################################################
set_target_properties(${PROJECT_NAME} PROPERTIES
        TARGET_NAME_DEBUG "FlightStripsPlugin"
        TARGET_NAME_RELEASE "FlightStripsPlugin"
)

################################################################################
# Output directory
################################################################################
set_target_properties(${PROJECT_NAME} PROPERTIES
        OUTPUT_DIRECTORY_DEBUG "${CMAKE_BINARY_DIR}/bin/"
        OUTPUT_DIRECTORY_RELEASE "${CMAKE_BINARY_DIR}/bin/"
        INTERPROCEDURAL_OPTIMIZATION_RELEASE "TRUE"
)

################################################################################
# MSVC runtime library
################################################################################
get_property(MSVC_RUNTIME_LIBRARY_DEFAULT TARGET ${PROJECT_NAME} PROPERTY MSVC_RUNTIME_LIBRARY)
string(CONCAT "MSVC_RUNTIME_LIBRARY_STR"
        $<$<CONFIG:Debug>:MultiThreadedDebugDLL>
        $<$<CONFIG:Release>:MultiThreadedDLL>
        $<$<NOT:$<OR:$<CONFIG:Debug>,$<CONFIG:Release>>>:${MSVC_RUNTIME_LIBRARY_DEFAULT}>
)
set_target_properties(${PROJECT_NAME} PROPERTIES MSVC_RUNTIME_LIBRARY ${MSVC_RUNTIME_LIBRARY_STR})
set_property(TARGET ${PROJECT_NAME} PROPERTY CXX_STANDARD 20)

################################################################################
# Include directories
################################################################################
target_include_directories(${PROJECT_NAME} PUBLIC
        "${CMAKE_CURRENT_SOURCE_DIR}/."
)

target_include_directories(${PROJECT_NAME} SYSTEM PRIVATE
        "${CMAKE_CURRENT_SOURCE_DIR}/../../third_party/asio"
        "${CMAKE_CURRENT_SOURCE_DIR}/../../third_party"
)


target_link_directories(${PROJECT_NAME} PUBLIC ${CMAKE_CURRENT_SOURCE_DIR}/../../lib)

################################################################################
# Compile definitions
################################################################################

target_compile_definitions(${PROJECT_NAME} PRIVATE
        "$<$<CONFIG:Debug>:_DEBUG>"
        "$<$<CONFIG:Release>:NDEBUG>"
        "WIN32"
        "_WINDOWS"
        "_USRDLL"
        "UNICODE"
        "_UNICODE"
        "CURL_STATICLIB;"
)

################################################################################
# Compile and link options
################################################################################
if (MSVC)
        target_compile_options(${PROJECT_NAME} PRIVATE
                $<$<CONFIG:Release>:
                /Oi;
                /Gy
                >
                /permissive-;
                /std:c++20;
                /sdl;
                /utf-8;
                ${DEFAULT_CXX_DEBUG_INFORMATION_FORMAT};
                /Zm200;
                ${DEFAULT_CXX_EXCEPTION_HANDLING}
        )
        target_link_options(${PROJECT_NAME} PRIVATE
                $<$<CONFIG:Debug>:
                /DEBUG:GHASH;
                /INCREMENTAL
                -flto=thin
                >
                $<$<CONFIG:Release>:
                /DEBUG:NONE;
                /OPT:REF;
                /OPT:ICF;
                /INCREMENTAL:NO
                >
                /NODEFAULTLIB:LIBCMT;
                /SUBSYSTEM:WINDOWS
                /SAFESEH:NO
        )
endif ()

################################################################################
# Dependencies
################################################################################
add_dependencies(${PROJECT_NAME}
        FlightStripsPluginUtils
)

# Link with other targets.
target_link_libraries(${PROJECT_NAME} PUBLIC
        FlightStripsPluginUtils
)

set(ADDITIONAL_LIBRARY_DEPENDENCIES
        "kernel32"
        "user32"
        "gdi32"
        "winspool"
        "Ws2_32"
        "Wldap32"
        "normaliz"
        "crypt32"
        "comdlg32"
        "advapi32"
        "shell32"
        "ole32"
        "oleaut32"
        "uuid"
        "odbc32"
        "odbccp32"
        "EuroScopePlugInDll"
        "Winmm"
        "Dwmapi"
        "gdiplus"
        "comctl32"
)

target_link_libraries(${PROJECT_NAME} PRIVATE libcrypto.lib libssl.lib libcurl_a.lib)

target_link_libraries(${PROJECT_NAME} PUBLIC ${ADDITIONAL_LIBRARY_DEPENDENCIES})
