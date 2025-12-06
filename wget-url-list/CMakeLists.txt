cmake_minimum_required(VERSION 3.10)
project(url_downloader LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

add_executable(url-downloader main.cpp)

if(MSVC)
  target_compile_options(url-downloader PRIVATE /W4 /permissive-)
else()
  target_compile_options(url-downloader PRIVATE -Wall -Wextra -Wpedantic)
endif()
