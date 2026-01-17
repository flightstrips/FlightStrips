set(PROJECT_NAME FlightStripsPluginUtils)

################################################################################
# Source groups
################################################################################
set(src__http
        http/Http.h
        http/Http.cpp)
source_group("src\\http" FILES ${src__http})

set(src__filesystem
        filesystem/FileSystem.h
        filesystem/FileSystem.cpp)
source_group("src\\filesystem" FILES ${src__filesystem})

set(ALL_FILES ${src__http} ${src__filesystem} Logger.hpp )

# Read version from file
file(READ "${CMAKE_SOURCE_DIR}/VERSION.txt" PLUGIN_VERSION)
string(STRIP "${PLUGIN_VERSION}" PLUGIN_VERSION)

# Configure the version header
configure_file(
        "${CMAKE_SOURCE_DIR}/src/plugin/plugin/Version.h.in"
        "${CMAKE_CURRENT_BINARY_DIR}/generated/Version.h"
        @ONLY
)

################################################################################
# Target
################################################################################
add_library(${PROJECT_NAME} STATIC ${ALL_FILES})

target_precompile_headers(${PROJECT_NAME} PRIVATE "$<$<COMPILE_LANGUAGE:CXX>:${CMAKE_CURRENT_SOURCE_DIR}/pch/pch.h>")

use_props(${PROJECT_NAME} "${CMAKE_CONFIGURATION_TYPES}" "${DEFAULT_CXX_PROPS}")
set(ROOT_NAMESPACE FlightStripsPluginUtils)

set_target_properties(${PROJECT_NAME} PROPERTIES
        VS_GLOBAL_KEYWORD "Win32Proj"
)

set_property(TARGET ${PROJECT_NAME} PROPERTY CXX_STANDARD 20)
################################################################################
# Output directory
################################################################################
set_target_properties(${PROJECT_NAME} PROPERTIES
        OUTPUT_DIRECTORY_DEBUG "${CMAKE_BINARY_DIR}/lib/"
        OUTPUT_DIRECTORY_RELEASE "${CMAKE_BINARY_DIR}/lib/"
        INTERPROCEDURAL_OPTIMIZATION_RELEASE "TRUE"
)


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

target_include_directories(${PROJECT_NAME} PUBLIC "${CMAKE_CURRENT_BINARY_DIR}/generated")


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
