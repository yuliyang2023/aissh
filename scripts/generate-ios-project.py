#!/usr/bin/env python3
"""Generate the checked-in Xcode project without installing a project generator."""
from pathlib import Path
import hashlib, json
root = Path(__file__).resolve().parents[1] / 'ios'
objects = {}
def uid(name): return hashlib.sha1(name.encode()).hexdigest()[:24].upper()
def add(identity, **values):
    key = uid(identity); objects[key] = values; return key
def plist(v, depth=0):
    if isinstance(v, dict): return '{\n' + ''.join('\t'*(depth+1)+json.dumps(str(k))+' = '+plist(x,depth+1)+';\n' for k,x in v.items()) + '\t'*depth+'}'
    if isinstance(v, list): return '(' + ', '.join(plist(x,depth) for x in v) + ')'
    return json.dumps(str(v))
packages = []
products = []
for name, url, version, libs, exact in [
    ('Citadel','https://github.com/orlandos-nl/Citadel.git','0.12.1',['Citadel'],True),
    ('SwiftTerm','https://github.com/migueldeicaza/SwiftTerm.git','1.20.0',['SwiftTerm'],True),
    ('NIOSSH','https://github.com/Wellz26/swift-nio-ssh.git','0.3.4',['NIOSSH'],True),
    ('NIO','https://github.com/apple/swift-nio.git','2.81.0',['NIOCore'],False),
    ('Crypto','https://github.com/apple/swift-crypto.git','3.12.3',['Crypto'],False),
]:
    pkg = add('package:'+name,isa='XCRemoteSwiftPackageReference',repositoryURL=url,requirement={'kind':'exactVersion' if exact else 'upToNextMajorVersion', 'version' if exact else 'minimumVersion':version})
    packages.append(pkg)
    for lib in libs: products.append(add('product:'+lib,isa='XCSwiftPackageProductDependency',package=pkg,productName=lib))
targets=[];groups=[];built=[]
for name, kind in [('WRDAISSH','application'),('WRDAISSHTests','bundle.unit-test'),('WRDAISSHUITests','bundle.ui-testing')]:
    files=[]; sources=[]; resources=[]
    for file in sorted((root/name).glob('*.swift')):
        ref=add(str(file.relative_to(root)),isa='PBXFileReference',lastKnownFileType='sourcecode.swift',path=file.name,sourceTree='<group>');files.append(ref)
        sources.append(add('build:'+name+file.name,isa='PBXBuildFile',fileRef=ref))
    if name=='WRDAISSH':
        for path, typ in [('Info.plist','text.plist.xml'),('Assets.xcassets','folder.assetcatalog'),('PrivacyInfo.xcprivacy','text.xml')]:
            ref=add('file:'+path,isa='PBXFileReference',lastKnownFileType=typ,path=path,sourceTree='<group>');files.append(ref)
            if path!='Info.plist': resources.append(add('resource:'+path,isa='PBXBuildFile',fileRef=ref))
    groups.append(add('group:'+name,isa='PBXGroup',children=files,path=name,sourceTree='<group>'))
    app=name=='WRDAISSH'
    product=add('built:'+name,isa='PBXFileReference',explicitFileType='wrapper.application' if app else 'wrapper.cfbundle',path=name+('.app' if app else '.xctest'),sourceTree='BUILT_PRODUCTS_DIR');built.append(product)
    phases=[add('sources:'+name,isa='PBXSourcesBuildPhase',buildActionMask=2147483647,files=sources,runOnlyForDeploymentPostprocessing=0),add('resources:'+name,isa='PBXResourcesBuildPhase',buildActionMask=2147483647,files=resources,runOnlyForDeploymentPostprocessing=0)]
    frameworks=[add('link:'+str(p),isa='PBXBuildFile',productRef=p) for p in products] if app else []
    phases.append(add('frameworks:'+name,isa='PBXFrameworksBuildPhase',buildActionMask=2147483647,files=frameworks,runOnlyForDeploymentPostprocessing=0))
    configs=[]
    for cfg in ['Debug','Release']:
        settings={'PRODUCT_BUNDLE_IDENTIFIER':'com.wrdtech.aissh.ios'+('' if app else '.'+name),'PRODUCT_NAME':'$(TARGET_NAME)','SWIFT_VERSION':'5.0','TARGETED_DEVICE_FAMILY':'1','IPHONEOS_DEPLOYMENT_TARGET':'17.0','CODE_SIGN_STYLE':'Automatic','GENERATE_INFOPLIST_FILE':'YES','SWIFT_EMIT_LOC_STRINGS':'YES','SWIFT_OPTIMIZATION_LEVEL':'-Onone' if cfg=='Debug' else '-O','SWIFT_ACTIVE_COMPILATION_CONDITIONS':'DEBUG' if cfg=='Debug' else '', 'SDKROOT':'iphoneos','SUPPORTED_PLATFORMS':'iphoneos iphonesimulator','ONLY_ACTIVE_ARCH':'YES' if cfg=='Debug' else 'NO'}
        if app: settings.update(INFOPLIST_FILE='WRDAISSH/Info.plist',ASSETCATALOG_COMPILER_APPICON_NAME='AppIcon',ASSETCATALOG_COMPILER_GLOBAL_ACCENT_COLOR_NAME='AccentColor',CURRENT_PROJECT_VERSION='1',MARKETING_VERSION='0.1.0',LD_RUNPATH_SEARCH_PATHS='$(inherited) @executable_path/Frameworks')
        elif kind=='bundle.unit-test': settings.update(TEST_HOST='$(BUILT_PRODUCTS_DIR)/WRDAISSH.app/$(BUNDLE_EXECUTABLE_FOLDER_PATH)/WRDAISSH',BUNDLE_LOADER='$(TEST_HOST)')
        else: settings['TEST_TARGET_NAME']='WRDAISSH'
        configs.append(add('config:'+name+cfg,isa='XCBuildConfiguration',name=cfg,buildSettings=settings))
    conf=add('configs:'+name,isa='XCConfigurationList',buildConfigurations=configs,defaultConfigurationIsVisible=0,defaultConfigurationName='Release')
    deps=[]
    if not app:
        proxy=add('proxy:'+name,isa='PBXContainerItemProxy',containerPortal=uid('project'),proxyType=1,remoteGlobalIDString=uid('target:WRDAISSH'),remoteInfo='WRDAISSH')
        deps.append(add('dep:'+name,isa='PBXTargetDependency',target=uid('target:WRDAISSH'),targetProxy=proxy))
    targets.append(add('target:'+name,isa='PBXNativeTarget',name=name,productName=name,productReference=product,productType='com.apple.product-type.'+kind,buildConfigurationList=conf,buildPhases=phases,buildRules=[],dependencies=deps,packageProductDependencies=products if app else []))
productsGroup=add('builtgroup',isa='PBXGroup',children=built,name='Products',sourceTree='<group>')
main=add('maingroup',isa='PBXGroup',children=groups+[productsGroup],sourceTree='<group>')
configs=[]
for cfg in ['Debug','Release']:
    configs.append(add('projectconfig:'+cfg,isa='XCBuildConfiguration',name=cfg,buildSettings={'CLANG_ENABLE_MODULES':'YES','CLANG_ENABLE_OBJC_ARC':'YES','ENABLE_TESTABILITY':'YES' if cfg=='Debug' else 'NO','DEBUG_INFORMATION_FORMAT':'dwarf' if cfg=='Debug' else 'dwarf-with-dsym','GCC_C_LANGUAGE_STANDARD':'gnu17','SWIFT_COMPILATION_MODE':'singlefile' if cfg=='Debug' else 'wholemodule'}))
conf=add('projectconfigs',isa='XCConfigurationList',buildConfigurations=configs,defaultConfigurationIsVisible=0,defaultConfigurationName='Release')
project=add('project',isa='PBXProject',attributes={'LastUpgradeCheck':'2600','TargetAttributes':{uid('target:WRDAISSH'):{'CreatedOnToolsVersion':'26.0'}}},buildConfigurationList=conf,compatibilityVersion='Xcode 14.0',developmentRegion='zh-Hans',hasScannedForEncodings=0,knownRegions=['zh-Hans','en','Base'],mainGroup=main,productRefGroup=productsGroup,projectDirPath='',projectRoot='',targets=targets,packageReferences=packages)
p=root/'WRDAISSH.xcodeproj';p.mkdir(exist_ok=True)
(p/'project.pbxproj').write_text('// !$*UTF8*$!\n'+plist({'archiveVersion':1,'classes':{},'objectVersion':56,'objects':objects,'rootObject':project})+'\n')
scheme=p/'xcshareddata/xcschemes';scheme.mkdir(parents=True,exist_ok=True)
def ref(name): return f'<BuildableReference BuildableIdentifier="primary" BlueprintIdentifier="{uid("target:"+name)}" BuildableName="{name}.{"app" if name=="WRDAISSH" else "xctest"}" BlueprintName="{name}" ReferencedContainer="container:WRDAISSH.xcodeproj"/>'
(scheme/'WRDAISSH.xcscheme').write_text(f'''<?xml version="1.0" encoding="UTF-8"?>
<Scheme LastUpgradeVersion="2600" version="1.3"><BuildAction parallelizeBuildables="YES" buildImplicitDependencies="YES"><BuildActionEntries><BuildActionEntry buildForTesting="YES" buildForRunning="YES" buildForProfiling="YES" buildForArchiving="YES" buildForAnalyzing="YES">{ref('WRDAISSH')}</BuildActionEntry></BuildActionEntries></BuildAction><TestAction buildConfiguration="Debug" selectedDebuggerIdentifier="Xcode.DebuggerFoundation.Debugger.LLDB" selectedLauncherIdentifier="Xcode.IDEFoundation.Launcher.LLDB" shouldUseLaunchSchemeArgsEnv="NO"><Testables><TestableReference skipped="NO">{ref('WRDAISSHTests')}</TestableReference><TestableReference skipped="NO">{ref('WRDAISSHUITests')}</TestableReference></Testables></TestAction><LaunchAction buildConfiguration="Debug" selectedDebuggerIdentifier="Xcode.DebuggerFoundation.Debugger.LLDB" selectedLauncherIdentifier="Xcode.IDEFoundation.Launcher.LLDB" launchStyle="0" useCustomWorkingDirectory="NO" ignoresPersistentStateOnLaunch="NO" debugDocumentVersioning="YES" allowLocationSimulation="YES"><BuildableProductRunnable runnableDebuggingMode="0">{ref('WRDAISSH')}</BuildableProductRunnable></LaunchAction><ProfileAction buildConfiguration="Release" shouldUseLaunchSchemeArgsEnv="YES" savedToolIdentifier="" useCustomWorkingDirectory="NO" debugDocumentVersioning="YES"><BuildableProductRunnable runnableDebuggingMode="0">{ref('WRDAISSH')}</BuildableProductRunnable></ProfileAction><AnalyzeAction buildConfiguration="Debug"/><ArchiveAction buildConfiguration="Release" revealArchiveInOrganizer="YES"/></Scheme>''')
print(p)
