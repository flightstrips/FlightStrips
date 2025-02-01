set(PROJECT_NAME FlightStripsCore)

################################################################################
# Source groups
################################################################################
set(src__plugin
        plugin/FlightStripsPlugin.h
        plugin/FlightStripsPlugin.cpp)
source_group("src\\plugin" FILES ${src__plugin})

set(src__filesystem
        filesystem/FileSystem.h
        filesystem/FileSystem.cpp)
source_group("src\\filesystem" FILES ${src__filesystem})

set(src__stands
        stands/Stand.h
        stands/Stand.cpp
        stands/StandService.h
        stands/StandService.cpp
        stands/StandsBootstrapper.cpp
        stands/StandsBootstrapper.h)
source_group("src\\stands" FILES ${src__stands})

set(src__bootstrap
        bootstrap/dllmain.cpp
        bootstrap/Container.h
        bootstrap/Container.cpp
        bootstrap/InitializePlugin.h
        bootstrap/InitializePlugin.cpp)
source_group("src\\bootstrap" FILES ${src__bootstrap})

set(src__configuration
        configuration/config.cpp
        configuration/config.h
        configuration/AppConfig.cpp
        configuration/AppConfig.h
        configuration/UserConfig.cpp
        configuration/UserConfig.h
        configuration/ConfigurationBootstrapper.cpp
        configuration/ConfigurationBootstrapper.h)
source_group("src\\configuration" FILES ${src__configuration})

set(src__handlers
        handlers/FlightPlanEventHandler.h
        handlers/FlightPlanEventHandlers.h
        handlers/FlightPlanEventHandlers.cpp
        handlers/RadarTargetEventHandler.h
        handlers/RadarTargetEventHandlers.cpp
        handlers/RadarTargetEventHandlers.h
        handlers/AirportRunwaysChangedEvent.h
        handlers/ControllerEventHandler.h
        handlers/ControllerEventHandlers.cpp
        handlers/ControllerEventHandlers.h
        handlers/TimedEventHandler.h
        handlers/TimedEventHandlers.cpp
        handlers/TimedEventHandlers.h
        handlers/AirportRunwaysChangedEventHandlers.cpp
        handlers/AirportRunwaysChangedEventHandlers.h
        handlers/ConnectionEventHandlers.cpp
        handlers/ConnectionEventHandlers.h
        handlers/ConnectionEventHandler.h)
source_group("src\\collections" FILES ${src__handlers})

set(src__flightplan
        flightplan/FlightPlanService.h
        flightplan/FlightPlanService.cpp
        flightplan/FlightPlan.h
        flightplan/FlightPlanBootstrapper.cpp
        flightplan/FlightPlanBootstrapper.h
)
source_group("src\\flightplan" FILES ${src__flightplan})

set(src__runway
        runway/RunwayService.cpp
        runway/RunwayService.h)
source_group("src\\runway" FILES ${src__runway})

set(src__authentication
        authentication/AuthenticationService.cpp
        authentication/AuthenticationService.h
        authentication/AuthenticationRedirectListener.cpp
        authentication/AuthenticationRedirectListener.h)
source_group("src\\authentication" FILES ${src__authentication})

set(src__graphics
        graphics/InfoScreen.cpp
        graphics/InfoScreen.h
        graphics/Colors.h
        graphics/Graphics.cpp
        graphics/Graphics.h)
source_group("src\\graphics" FILES ${src__graphics})

set(src__http
        http/Http.h
        http/Http.cpp)
source_group("src\\http" FILES ${src__http})

set(src__websocket
        websocket/WebSocket.h
        websocket/WebSocket.cpp
        websocket/WebSocketService.cpp
        websocket/WebSocketService.h
        websocket/Events.h)
source_group("src\\websocket" FILES ${src__websocket})

set(ALL_FILES
        ${src__plugin}
        ${src__filesystem}
        ${src__configuration}
        ${src__stands}
        ${src__bootstrap}
        ${src__handlers}
        ${src__flightplan}
        ${src__runway}
        ${src__authentication}
        ${src__graphics}
        ${src__http}
        ${src__websocket}
        Constants.h
        Logger.cpp
        Logger.h)

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


add_custom_command(
        TARGET ${PROJECT_NAME} POST_BUILD
        COMMAND ${CMAKE_COMMAND} -E copy
        ${CMAKE_SOURCE_DIR}/src/config.ini
        ${PROJECT_BINARY_DIR}/bin/flightstrips_config.ini)
