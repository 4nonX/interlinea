cmake_minimum_required(VERSION 3.21)

project(Interlinea VERSION 0.1.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_AUTOMOC ON)

find_package(Qt6 6.4 REQUIRED COMPONENTS Core Gui Widgets Network Test)

# Newer macOS SDKs have no AGL framework, which Qt's OpenGL wrapper still
# names; nothing in the app uses it.
if(APPLE AND TARGET WrapOpenGL::WrapOpenGL)
    get_target_property(gl_libs WrapOpenGL::WrapOpenGL INTERFACE_LINK_LIBRARIES)
    if(gl_libs)
        list(FILTER gl_libs EXCLUDE REGEX "AGL")
        set_target_properties(WrapOpenGL::WrapOpenGL PROPERTIES INTERFACE_LINK_LIBRARIES "${gl_libs}")
    endif()
endif()

option(INTERLINEA_BUILD_CORE "Build interlinea-core with Go next to the app" ON)
set(INTERLINEA_VERSION "${PROJECT_VERSION}" CACHE STRING "Version shown by the app")

# The app's code, shared by the app and the tests.
add_library(interlinea_app STATIC
    src/apierror.cpp
    src/apiclient.cpp
    src/eventfeed.cpp
    src/secretstore.cpp
    src/corelauncher.cpp
    src/connection.cpp
    src/connectionmanager.cpp
    src/providersignin.cpp
    src/singleinstance.cpp
    src/navigationrail.cpp
    src/signindialog.cpp
    src/settingspage.cpp
    src/mainwindow.cpp
    src/languages.cpp
    src/projectsmodel.cpp
    src/projectform.cpp
    src/newprojectpage.cpp
    src/projectpage.cpp
    src/projectspage.cpp
    src/theme.cpp
    src/icons.cpp
    src/progressdelegate.cpp
    src/segmentcontent.cpp
    src/segmentmodel.cpp
    src/targeteditor.cpp
    src/editorpage.cpp
    src/bands.cpp
    src/flowlayout.cpp
    src/webui.cpp
    src/accesspanel.cpp
    src/accountspage.cpp
    src/alignmentdialog.cpp
    src/memoriespage.cpp
    src/options.cpp
    src/roles.cpp
    src/reviewpage.cpp
    src/termbasedialogs.cpp
    src/termdata.cpp
    src/terminologypage.cpp
    src/words.cpp
)
# The licence and the copyright notice are in the app, which shows them.
qt_add_resources(interlinea_app "legal" PREFIX "/legal" BASE .. FILES ../LICENSE ../NOTICE)
target_include_directories(interlinea_app PUBLIC src)
target_compile_definitions(interlinea_app PUBLIC INTERLINEA_VERSION="${INTERLINEA_VERSION}")
target_link_libraries(interlinea_app PUBLIC Qt6::Core Qt6::Gui Qt6::Widgets Qt6::Network)

# Sign-ins go to the system's keychain.
if(WIN32)
    target_link_libraries(interlinea_app PRIVATE advapi32)
elseif(APPLE)
    target_link_libraries(interlinea_app PRIVATE "-framework Security" "-framework CoreFoundation")
else()
    find_package(PkgConfig)
    if(PkgConfig_FOUND)
        pkg_check_modules(LIBSECRET IMPORTED_TARGET libsecret-1)
    endif()
    if(LIBSECRET_FOUND)
        target_link_libraries(interlinea_app PRIVATE PkgConfig::LIBSECRET)
        target_compile_definitions(interlinea_app PRIVATE INTERLINEA_LIBSECRET=1)
    endif()
endif()

if(MSVC)
    target_compile_options(interlinea_app PUBLIC /W4 /permissive- /utf-8)
else()
    target_compile_options(interlinea_app PUBLIC -Wall -Wextra -Wpedantic)
endif()

qt_add_executable(interlinea WIN32 MACOSX_BUNDLE src/main.cpp)
target_link_libraries(interlinea PRIVATE interlinea_app)
set_target_properties(interlinea PROPERTIES
    OUTPUT_NAME Interlinea
    MACOSX_BUNDLE_BUNDLE_NAME Interlinea
    MACOSX_BUNDLE_GUI_IDENTIFIER org.interlinea.desktop
    MACOSX_BUNDLE_BUNDLE_VERSION ${PROJECT_VERSION}
    MACOSX_BUNDLE_SHORT_VERSION_STRING ${PROJECT_VERSION}
    MACOSX_BUNDLE_COPYRIGHT "Copyright (C) 2026 Dan Dreßen. GNU Affero General Public License, version 3.")

# The app's icon: in the program file on Windows, in the bundle on macOS,
# and as the icon of its windows everywhere.
qt_add_resources(interlinea "appicon" PREFIX "/" BASE packaging FILES packaging/interlinea.png)
if(WIN32)
    enable_language(RC)
    target_sources(interlinea PRIVATE packaging/interlinea.rc)
elseif(APPLE)
    target_sources(interlinea PRIVATE packaging/interlinea.icns)
    set_source_files_properties(packaging/interlinea.icns PROPERTIES MACOSX_PACKAGE_LOCATION Resources)
    set_target_properties(interlinea PROPERTIES MACOSX_BUNDLE_ICON_FILE interlinea.icns)
endif()

# interlinea-core goes next to the app (inside the bundle on macOS), where
# the app looks for it.
if(INTERLINEA_BUILD_CORE)
    find_program(GO_EXECUTABLE go REQUIRED)
    if(APPLE)
        set(CORE_DIR "$<TARGET_BUNDLE_CONTENT_DIR:interlinea>/MacOS")
    else()
        set(CORE_DIR "$<TARGET_FILE_DIR:interlinea>")
    endif()
    add_custom_target(interlinea_core ALL
        COMMAND ${GO_EXECUTABLE} build -trimpath -ldflags "-s -w -X main.version=${INTERLINEA_VERSION}" -o "${CORE_DIR}/interlinea-core${CMAKE_EXECUTABLE_SUFFIX}" ./cmd/interlinea-core
        WORKING_DIRECTORY "${CMAKE_CURRENT_SOURCE_DIR}/.."
        COMMENT "Building interlinea-core"
        VERBATIM)
    add_dependencies(interlinea_core interlinea)
endif()

enable_testing()
foreach(name apiclient eventfeed secretstore providersignin connections ui projects editor memories terminology endtoend)
    qt_add_executable(tst_${name} tests/tst_${name}.cpp tests/testcore.cpp)
    target_include_directories(tst_${name} PRIVATE tests)
    target_link_libraries(tst_${name} PRIVATE interlinea_app Qt6::Test)
    # One small document of every format, shared with the browser tests.
    target_compile_definitions(tst_${name} PRIVATE INTERLINEA_FIXTURES="${CMAKE_CURRENT_SOURCE_DIR}/../web/e2e/fixtures")
    add_test(NAME ${name} COMMAND tst_${name})
    set(test_env "QT_QPA_PLATFORM=offscreen")
    if(INTERLINEA_BUILD_CORE)
        # The tests run against the core built next to the app.
        add_dependencies(tst_${name} interlinea_core)
        list(APPEND test_env "INTERLINEA_CORE=${CORE_DIR}/interlinea-core${CMAKE_EXECUTABLE_SUFFIX}")
    endif()
    set_tests_properties(${name} PROPERTIES ENVIRONMENT "${test_env}" TIMEOUT 180)
endforeach()
