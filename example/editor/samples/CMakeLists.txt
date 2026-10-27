cmake_minimum_required(VERSION 3.20)
project(EditorExample LANGUAGES CXX)

#[=[
A small build file used only as an editor sample.
]=]
set(SOURCES main.cpp editor.cpp)
add_executable(editor ${SOURCES})
target_compile_features(editor PRIVATE cxx_std_20)
target_compile_definitions(editor PRIVATE
  $<$<CONFIG:Debug>:EDITOR_DEBUG=1>
)
message(STATUS "Building ${PROJECT_NAME}")
