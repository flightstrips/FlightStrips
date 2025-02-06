set(PROJECT_NAME FlightStripsCoreTests)

################################################################################
# Source groups
################################################################################

set(ALL_FILES
        doctest.cpp
        doctest.h
        $<TARGET_OBJECTS:FlightStripsCore>
        AuthenticationServiceTest.cpp)

################################################################################
# Target
################################################################################
#add_library(${PROJECT_NAME} SHARED ${ALL_FILES})
add_executable(${PROJECT_NAME} ${ALL_FILES})
add_test(NAME ${PROJECT_NAME} COMMAND ${PROJECT_NAME})

use_props(${PROJECT_NAME} "${CMAKE_CONFIGURATION_TYPES}" "${DEFAULT_CXX_PROPS}")
set(ROOT_NAMESPACE FlightStripsTest)

set_target_properties(${PROJECT_NAME} PROPERTIES
        VS_GLOBAL_KEYWORD "Win32Proj"
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
        "../../src/plugin"
)


target_include_directories(${PROJECT_NAME} SYSTEM PRIVATE
        "../../third_party/asio"
        "../../third_party"
)

target_link_directories(${PROJECT_NAME} PUBLIC ../../lib)

################################################################################
# Compile definitions
################################################################################

target_compile_definitions(${PROJECT_NAME} PRIVATE
        "$<$<CONFIG:Debug>:_DEBUG>"
        "$<$<CONFIG:Release>:NDEBUG>"
        "WIN32"
        "_CONSOLE"
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
                /SUBSYSTEM:CONSOLE
                /SAFESEH:NO
        )
endif ()

################################################################################
# Dependencies
################################################################################

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

