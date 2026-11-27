cmake_minimum_required(VERSION 3.21)

project(Interlinea VERSION 0.5.1 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_AUTOMOC ON)

find_package(Qt6 6.8 REQUIRED COMPONENTS Core Gui Widgets Network Quick QuickControls2 Test)

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

# The app's code without its interface, shared by the app and the tests:
# the connections to the core and to team servers, sign-ins, the live
# feed, the interface languages, and what the interface asks of C++.
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
    src/interfacelanguage.cpp
    src/languages.cpp
    src/options.cpp
    src/roles.cpp
    src/words.cpp
    src/colors.cpp
    src/icons.cpp
    src/segmentcontent.cpp
    src/documentpreview.cpp
)
# The licence and the copyright notice are in the app, which shows them.
qt_add_resources(interlinea_app "legal" PREFIX "/legal" BASE .. FILES ../LICENSE ../NOTICE)
# The interface translations, shared with the web app.
qt_add_resources(interlinea_app "i18n" PREFIX "/i18n" BASE ../i18n FILES ../i18n/de.json ../i18n/ko.json)
target_include_directories(interlinea_app PUBLIC src)
target_compile_definitions(interlinea_app PUBLIC INTERLINEA_VERSION="${INTERLINEA_VERSION}")
target_link_libraries(interlinea_app PUBLIC Qt6::Core Qt6::Gui Qt6::Widgets Qt6::Network)

# The interface in Qt Quick: the app's own controls in the web app's
# design (qml/controls), its parts and pages (qml/parts, qml/pages), and
# the bridge to the C++ side (src/ui). A library the app and the tests
# both load the interface from.
qt_policy(SET QTP0001 NEW)
qt_policy(SET QTP0004 NEW)
qt_add_library(interlinea_ui STATIC)
file(GLOB interlinea_qml RELATIVE ${CMAKE_CURRENT_SOURCE_DIR} CONFIGURE_DEPENDS qml/*.qml qml/controls/*.qml qml/parts/*.qml qml/pages/*.qml)
set_source_files_properties(qml/Theme.qml qml/Fmt.qml qml/Api.qml qml/Words.qml qml/Roles.qml qml/TermData.qml qml/Fields.qml qml/Bands.qml PROPERTIES QT_QML_SINGLETON_TYPE TRUE)
qt_add_qml_module(interlinea_ui
    URI Interlinea
    VERSION 1.0
    # Not "Interlinea" in the build folder, where the app's program is.
    OUTPUT_DIRECTORY qml/Interlinea
    SOURCES src/ui/backend.h src/ui/backend.cpp src/ui/iconprovider.h src/ui/iconprovider.cpp src/ui/segmentdocument.h src/ui/segmentdocument.cpp
            src/ui/shothook.h src/ui/shothook.cpp
    QML_FILES ${interlinea_qml}
)
target_include_directories(interlinea_ui PUBLIC src/ui)
target_link_libraries(interlinea_ui PUBLIC interlinea_app Qt6::Quick Qt6::QuickControls2)

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

qt_add_executable(interlinea WIN32 MACOSX_BUNDLE src/ui/main.cpp)
target_link_libraries(interlinea PRIVATE interlinea_ui interlinea_uiplugin)
set_target_properties(interlinea PROPERTIES
    OUTPUT_NAME Interlinea
    MACOSX_BUNDLE_BUNDLE_NAME Interlinea
    MACOSX_BUNDLE_GUI_IDENTIFIER dev.interlinea.Interlinea
    MACOSX_BUNDLE_BUNDLE_VERSION ${PROJECT_VERSION}
    MACOSX_BUNDLE_SHORT_VERSION_STRING ${PROJECT_VERSION}
    MACOSX_BUNDLE_COPYRIGHT "Copyright (C) 2026 Dan Dreßen. GNU Affero General Public License, version 3.")


# The app's icon: in the program file on Windows, in the bundle on macOS,
# and as the icon of its windows everywhere.
qt_add_resources(interlinea "appicon" PREFIX "/" BASE packaging FILES packaging/interlinea.png)
if(WIN32)
    enable_language(RC)
    # The resources carry the version, in numbers (1,2,3,0) and as written.
    string(REGEX MATCH "^([0-9]+)\\.([0-9]+)\\.([0-9]+)" _version "${INTERLINEA_VERSION}")
    if(_version)
        set(INTERLINEA_VERSION_COMMAS "${CMAKE_MATCH_1},${CMAKE_MATCH_2},${CMAKE_MATCH_3},0")
    else()
        set(INTERLINEA_VERSION_COMMAS "0,0,0,0")
    endif()
    set(INTERLINEA_ICON "${CMAKE_CURRENT_SOURCE_DIR}/packaging/interlinea.ico")
    configure_file(packaging/interlinea.rc.in "${CMAKE_CURRENT_BINARY_DIR}/interlinea.rc" @ONLY)
    target_sources(interlinea PRIVATE "${CMAKE_CURRENT_BINARY_DIR}/interlinea.rc")
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

# The spelling dictionaries that come with Interlinea ("make dictionaries"
# puts them in the web client's folder) go where the core finds them: next
# to it, or in the bundle's Resources on macOS. Without them the app has
# only the dictionaries the person adds.
set(INTERLINEA_DICTIONARIES "${CMAKE_CURRENT_SOURCE_DIR}/../web/public/dictionaries" CACHE PATH "Folder of the included spelling dictionaries")
if(EXISTS "${INTERLINEA_DICTIONARIES}/index.json")
    if(APPLE)
        set(DICT_DIR "$<TARGET_BUNDLE_CONTENT_DIR:interlinea>/Resources/dictionaries")
    else()
        set(DICT_DIR "$<TARGET_FILE_DIR:interlinea>/dictionaries")
    endif()
    add_custom_command(TARGET interlinea POST_BUILD
        COMMAND ${CMAKE_COMMAND} -E copy_directory "${INTERLINEA_DICTIONARIES}" "${DICT_DIR}"
        COMMENT "Copying the spelling dictionaries"
        VERBATIM)
else()
    message(STATUS "No spelling dictionaries in ${INTERLINEA_DICTIONARIES}: run make dictionaries to include them")
endif()

enable_testing()
foreach(name apiclient eventfeed secretstore providersignin connections interfacelanguage ui editor memories)
    qt_add_executable(tst_${name} tests/tst_${name}.cpp tests/testcore.cpp)
    target_include_directories(tst_${name} PRIVATE tests)
    target_link_libraries(tst_${name} PRIVATE interlinea_ui interlinea_uiplugin Qt6::Test)
    # One small document of every format, shared with the browser tests.
    target_compile_definitions(tst_${name} PRIVATE INTERLINEA_FIXTURES="${CMAKE_CURRENT_SOURCE_DIR}/../web/e2e/fixtures")
    add_test(NAME ${name} COMMAND tst_${name})
    # A small folder of included dictionaries, the same in every run.
    # Without a screen; on Windows on the real one, the only way the
    # windows get the system's fonts there (offscreen has none, and draws
    # every letter as a box), so the layout check measures real text.
    if(WIN32)
        set(test_platform "windows")
    else()
        set(test_platform "offscreen")
    endif()
    set(test_env "QT_QPA_PLATFORM=${test_platform}" "INTERLINEA_DICTIONARIES=${CMAKE_CURRENT_SOURCE_DIR}/tests/dictionaries")
    if(INTERLINEA_BUILD_CORE)
        # The tests run against the core built next to the app.
        add_dependencies(tst_${name} interlinea_core)
        list(APPEND test_env "INTERLINEA_CORE=${CORE_DIR}/interlinea-core${CMAKE_EXECUTABLE_SUFFIX}")
    endif()
    set_tests_properties(${name} PROPERTIES ENVIRONMENT "${test_env}" TIMEOUT 180)
    # Again at 150% scaling, as many screens are set: the layout check
    # finds what only breaks there. Not the end-to-end test: it works on
    # one team server, whose state a second run would find changed.
    if(NOT name STREQUAL "endtoend")
        add_test(NAME ${name}_150 COMMAND tst_${name})
        set_tests_properties(${name}_150 PROPERTIES ENVIRONMENT "${test_env};QT_SCALE_FACTOR=1.5" TIMEOUT 180)
    endif()
endforeach()
