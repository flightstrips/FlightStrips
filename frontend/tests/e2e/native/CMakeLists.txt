cmake_minimum_required(VERSION 3.24)
project(AMANLiveDecoder LANGUAGES CXX)
set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
include(FetchContent)
set(protobuf_BUILD_TESTS OFF CACHE BOOL "" FORCE)
set(protobuf_BUILD_CONFORMANCE OFF CACHE BOOL "" FORCE)
set(protobuf_BUILD_EXAMPLES OFF CACHE BOOL "" FORCE)
set(protobuf_BUILD_PROTOC_BINARIES OFF CACHE BOOL "" FORCE)
set(protobuf_BUILD_LIBPROTOC OFF CACHE BOOL "" FORCE)
set(protobuf_BUILD_SHARED_LIBS OFF CACHE BOOL "" FORCE)
set(protobuf_WITH_ZLIB OFF CACHE BOOL "" FORCE)
set(ABSL_PROPAGATE_CXX_STD ON CACHE BOOL "" FORCE)
# Match the production plugin's committed C++ gencode and runtime exactly.
FetchContent_Declare(protobuf
  GIT_REPOSITORY https://github.com/protocolbuffers/protobuf.git
  GIT_TAG v26.1 GIT_SHALLOW TRUE GIT_SUBMODULES_RECURSE TRUE)
FetchContent_GetProperties(protobuf)
if(NOT protobuf_POPULATED)
  FetchContent_Populate(protobuf)
  add_subdirectory("${protobuf_SOURCE_DIR}" "${protobuf_BINARY_DIR}" EXCLUDE_FROM_ALL)
endif()
get_filename_component(REPOSITORY "${CMAKE_CURRENT_LIST_DIR}/../../../.." ABSOLUTE)
set(PLUGIN "${REPOSITORY}/euroscope-plugin")
add_executable(aman-live-decoder live-decoder.cpp
  "${PLUGIN}/src/plugin/aman/AMANGainLossStore.cpp"
  "${PLUGIN}/src/plugin/websocket/generated/proto/euroscope.pb.cc")
target_include_directories(aman-live-decoder PRIVATE
  "${PLUGIN}/src/plugin" "${PLUGIN}/src/plugin/websocket/generated"
  "${PLUGIN}/src/util" "${PLUGIN}/third_party")
target_link_libraries(aman-live-decoder PRIVATE protobuf::libprotobuf-lite)
if(MSVC)
  target_compile_options(aman-live-decoder PRIVATE /utf-8)
  set_property(TARGET aman-live-decoder PROPERTY MSVC_RUNTIME_LIBRARY "MultiThreaded$<$<CONFIG:Debug>:Debug>")
endif()
