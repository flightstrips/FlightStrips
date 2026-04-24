set(PROJECT_NAME FlightStripsCoreTests)

################################################################################
# Fetch GoogleTest
################################################################################
include(FetchContent)
FetchContent_Declare(
    googletest
    GIT_REPOSITORY https://github.com/google/googletest.git
    GIT_TAG        v1.14.0
)
set(gtest_force_shared_crt ON CACHE BOOL "" FORCE)
FetchContent_MakeAvailable(googletest)

################################################################################
# Source groups
################################################################################

set(ALL_FILES
        $<TARGET_OBJECTS:FlightStripsPluginCore>

        # Authentication
        authentication/AuthenticationServiceTest.cpp

        # FlightPlan
        flightplan/RouteServiceTest.cpp
        flightplan/FlightPlanServiceTest.cpp

        # Configuration
        configuration/AppConfigTest.cpp
        configuration/UserConfigTest.cpp

        # Stands
        stands/StandServiceTest.cpp
        stands/StandTest.cpp

        # Runway
        runway/RunwayServiceTest.cpp

        # Controller
        controller/ControllerServiceTest.cpp

        # Messages
        messages/CdmReadyTriggerTest.cpp
        messages/MessageServiceTest.cpp

        # Plugin
        plugin/AirportResolutionTest.cpp
        plugin/LookupAuditTest.cpp

        # Handlers
        handlers/FlightPlanEventHandlersTest.cpp
        handlers/RadarTargetEventHandlersTest.cpp
        handlers/ControllerEventHandlersTest.cpp
        handlers/TimedEventHandlersTest.cpp
        handlers/AirportRunwaysChangedEventHandlersTest.cpp
        handlers/ConnectionEventHandlersTest.cpp
        handlers/MessageHandlersTest.cpp
        handlers/AuthenticationEventHandlersTest.cpp

        # Tag Items
        tag_items/CdmStateHandlerTest.cpp
        tag_items/DeIceHandlerTest.cpp
        tag_items/ClearanceStatusHandlerTest.cpp

        # Graphics
        graphics/InfoPanelTest.cpp
        graphics/PdcPopupTest.cpp

        # WebSocket
        websocket/WebSocketServiceTest.cpp
)

################################################################################
# Target
################################################################################
add_executable(${PROJECT_NAME} ${ALL_FILES})

use_props(${PROJECT_NAME} "${CMAKE_CONFIGURATION_TYPES}" "${DEFAULT_CXX_PROPS}")
set(ROOT_NAMESPACE FlightStripsTest)

target_precompile_headers(${PROJECT_NAME} PRIVATE "$<$<COMPILE_LANGUAGE:CXX>:${CMAKE_CURRENT_SOURCE_DIR}/../../src/plugin/pch/pch.h>")

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
        "${CMAKE_CURRENT_SOURCE_DIR}"
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
                /utf-8;
                ${DEFAULT_CXX_DEBUG_INFORMATION_FORMAT};
                /Zm200;
                ${DEFAULT_CXX_EXCEPTION_HANDLING}
        )
        target_link_options(${PROJECT_NAME} PRIVATE
                $<$<CONFIG:Debug>:
                /DEBUG;
                /INCREMENTAL
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
add_dependencies(${PROJECT_NAME}
        FlightStripsPluginUtils
)

# Link with GoogleTest and GoogleMock
target_link_libraries(${PROJECT_NAME} PRIVATE
        GTest::gtest_main
        GTest::gmock
        GTest::gmock_main
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

################################################################################
# Runtime DLLs — copy EuroScope DLLs next to the test exe at build time
#
# FlightStripsCoreTests.exe depends on:
#   EuroScopePlugInDll.dll, libcrypto-1_1.dll, libssl-1_1.dll
#
# Search order:
#   1. Local EuroScope installation (developer machines)
#   2. euroscope-plugin/bin/ in the repo (committed for CI agents)
################################################################################
set(EUROSCOPE_DLL_CANDIDATES
    "C:/Program Files (x86)/EuroScope"
    "C:/Program Files/EuroScope"
)
foreach(CANDIDATE ${EUROSCOPE_DLL_CANDIDATES})
    if(IS_DIRECTORY "${CANDIDATE}")
        set(EUROSCOPE_INSTALL_DIR "${CANDIDATE}")
        break()
    endif()
endforeach()

# Fall back to the committed bin/ directory inside the repo when EuroScope
# is not installed (e.g. on a CI agent).
if(NOT DEFINED EUROSCOPE_INSTALL_DIR)
    set(EUROSCOPE_INSTALL_DIR "${CMAKE_CURRENT_SOURCE_DIR}/../../bin")
endif()

foreach(DLL EuroScopePlugInDll.dll libcrypto-1_1.dll libssl-1_1.dll)
    add_custom_command(TARGET ${PROJECT_NAME} POST_BUILD
        COMMAND ${CMAKE_COMMAND} -E copy_if_different
            "${EUROSCOPE_INSTALL_DIR}/${DLL}"
            "${CMAKE_BINARY_DIR}/bin/${DLL}"
        COMMENT "Copying ${DLL} from ${EUROSCOPE_INSTALL_DIR} to bin/"
    )
endforeach()

################################################################################
# Test discovery
################################################################################
include(GoogleTest)

gtest_discover_tests(${PROJECT_NAME}
    WORKING_DIRECTORY "${CMAKE_BINARY_DIR}/bin"
)
