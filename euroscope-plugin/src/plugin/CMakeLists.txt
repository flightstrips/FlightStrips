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
        stands/StandService.cpp stands/StandsBootstrapper.cpp stands/StandsBootstrapper.h)
source_group("src\\stands" FILES ${src__stands})

set(src__network
        network/ConnectedClient.cpp network/ConnectedClient.h network/Server.cpp network/Server.h network/NetworkService.cpp network/NetworkService.h network/NetworkBootstrapper.cpp network/NetworkBootstrapper.h)
source_group("src\\network" FILES ${src__network})

set(src__bootstrap
        bootstrap/dllmain.cpp
        bootstrap/Container.h
        bootstrap/Container.cpp
        bootstrap/InitializePlugin.h
        bootstrap/InitializePlugin.cpp)
source_group("src\\bootstrap" FILES ${src__bootstrap})

set(src__euroscope
        euroscope/JsonConversion.hpp
)
source_group("src\\euroscope" FILES ${src__euroscope})

set(src__handlers
        handlers/FlightPlanEventHandler.h
        handlers/FlightPlanEventHandlers.h
        handlers/FlightPlanEventHandlers.cpp handlers/RadarTargetEventHandler.h handlers/RadarTargetEventHandlers.cpp handlers/RadarTargetEventHandlers.h handlers/AirportRunwaysChangedEvent.h network/MessageHandler.cpp network/MessageHandler.h
        handlers/ControllerEventHandler.h
        handlers/ControllerEventHandlers.cpp
        handlers/ControllerEventHandlers.h)
source_group("src\\collections" FILES ${src__handlers})

set(src__flightplan
        flightplan/FlightPlanService.h
        flightplan/FlightPlanService.cpp flightplan/FlightPlan.h flightplan/FlightPlanBootstrapper.cpp flightplan/FlightPlanBootstrapper.h)
source_group("src\\flightplan" FILES ${src__flightplan})

set(src__runway
        runway/ActiveRunway.h)
source_group("src\\runway" FILES ${src__runway})

set(ALL_FILES
    ${src__plugin}
    ${src__filesystem}
    ${src__stands}
    ${src__network}
    ${src__bootstrap}
    ${src__euroscope}
    ${src__handlers}
    ${src__flightplan}
    ${src__runway}
        Constants.h)

################################################################################
# Target
################################################################################
add_library(${PROJECT_NAME} SHARED ${ALL_FILES})
set_target_properties(${PROJECT_NAME} PROPERTIES COMPILE_FLAGS "-m32" LINK_FLAGS "-m32")

target_precompile_headers(${PROJECT_NAME} PRIVATE "$<$<COMPILE_LANGUAGE:CXX>:${CMAKE_CURRENT_SOURCE_DIR}/pch/pch.h>")

use_props(${PROJECT_NAME} "${CMAKE_CONFIGURATION_TYPES}" "${DEFAULT_CXX_PROPS}")
set(ROOT_NAMESPACE FlightStrips)

set_target_properties(${PROJECT_NAME} PROPERTIES
        VS_GLOBAL_KEYWORD "Win32Proj"
        )
#############################################1###################################
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
        )
set_target_properties(${PROJECT_NAME} PROPERTIES
        INTERPROCEDURAL_OPTIMIZATION_RELEASE "TRUE"
        )
################################################################################
# MSVC runtime library
################################################################################
get_property(MSVC_RUNTIME_LIBRARY_DEFAULT TARGET ${PROJECT_NAME} PROPERTY MSVC_RUNTIME_LIBRARY)
string(CONCAT "MSVC_RUNTIME_LIBRARY_STR"
        $<$<CONFIG:Debug>:
        MultiThreadedDebugDLL
        >
        $<$<CONFIG:Release>:
        MultiThreadedDLL
        >
        $<$<NOT:$<OR:$<CONFIG:Debug>,$<CONFIG:Release>>>:${MSVC_RUNTIME_LIBRARY_DEFAULT}>
        )
set_target_properties(${PROJECT_NAME} PROPERTIES MSVC_RUNTIME_LIBRARY ${MSVC_RUNTIME_LIBRARY_STR})
set_property(TARGET ${PROJECT_NAME} PROPERTY CXX_STANDARD 20)
################################################################################
# Include directories
################################################################################
target_include_directories(${PROJECT_NAME} PUBLIC
        "${CMAKE_CURRENT_SOURCE_DIR}/.;"
        )

target_include_directories(${PROJECT_NAME} PRIVATE
        "${CMAKE_CURRENT_SOURCE_DIR}/../../third_party"
        )

target_link_directories(
        ${PROJECT_NAME} PUBLIC
        ${CMAKE_CURRENT_SOURCE_DIR}/../../lib
)

################################################################################
# Compile definitions
################################################################################
target_compile_definitions(${PROJECT_NAME} PRIVATE
        "$<$<CONFIG:Debug>:"
        "_DEBUG;"
        ">"
        "$<$<CONFIG:Release>:"
        "NDEBUG;"
        ">"
        "WIN32;"
        "_WINDOWS;"
        "_USRDLL;"
        "UNICODE;"
        "_UNICODE"
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
            /W4;
            /WX;
            ${DEFAULT_CXX_DEBUG_INFORMATION_FORMAT};
            /Zm200;
            ${DEFAULT_CXX_EXCEPTION_HANDLING}
            -Wno-unused-parameter # Lots of interfaces don't use everything
            -Wno-missing-field-initializers # Windows has loads of this sadly
            -ftime-trace
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
            /ALLOWISOLATION
            )
endif ()

################################################################################
# Dependencies
################################################################################

set(ADDITIONAL_LIBRARY_DEPENDENCIES
        "kernel32;"
        "user32;"
        "gdi32;"
        "winspool;"
        "Ws2_32;"
        "Wldap32;"
        "normaliz;"
        "crypt32;"
        "comdlg32;"
        "advapi32;"
        "shell32;"
        "ole32;"
        "oleaut32;"
        "uuid;"
        "odbc32;"
        "odbccp32;"
        "EuroScopePlugInDll;"
        "Winmm;"
        "Dwmapi;"
        "gdiplus;"
        "comctl32"
        )

target_link_libraries(${PROJECT_NAME} PUBLIC "${ADDITIONAL_LIBRARY_DEPENDENCIES}")
target_link_libraries(${PROJECT_NAME} PRIVATE nlohmann_json::nlohmann_json)

# Post-build copy the EuroScope binary
add_custom_command(TARGET ${PROJECT_NAME} POST_BUILD
        COMMAND ${CMAKE_COMMAND} -E copy "${CMAKE_CURRENT_SOURCE_DIR}/../../lib/EuroScopePlugInDll.dll" "${PROJECT_BINARY_DIR}/bin/EuroScopePlugInDll.dll"
        COMMENT "Copied EuroScope shared library to ${PROJECT_BINARY_DIR}/bin/EuroScopePlugInDll.dll"
        )

