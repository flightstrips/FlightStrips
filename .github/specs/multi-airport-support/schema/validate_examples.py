"""Validate the proposed config contract and the lossless EKCH migration example.

Install requirements.txt, then run this file. --refresh-ekch regenerates only
the worked example from the current sources; it does not change runtime
configuration. --dependency-path supports an isolated dependency installation.
"""
from __future__ import annotations

import argparse
import copy
import hashlib
import json
import os
import re
import sys
from pathlib import Path

PARSER = argparse.ArgumentParser(description=__doc__)
PARSER.add_argument('--dependency-path', type=Path)
PARSER.add_argument('--refresh-ekch', action='store_true')
PARSER.add_argument('--write-report', action='store_true')
PARSER.add_argument('files', nargs='*')
ARGS = PARSER.parse_args()
if ARGS.dependency_path:
    sys.path.insert(0, str(ARGS.dependency_path.resolve()))
try:
    import yaml
    from jsonschema import Draft202012Validator
except ImportError as exc:
    raise SystemExit('Install schema/requirements.txt or use --dependency-path') from exc

HERE = Path(__file__).resolve().parent
DESIGN = HERE.parent
ROOT = HERE.parents[3]
SCHEMA = json.loads((HERE / 'airport-configuration.schema.json').read_text(encoding='utf-8'))
Draft202012Validator.check_schema(SCHEMA)
VALIDATOR = Draft202012Validator(SCHEMA)


class ConfigError(ValueError):
    pass


class UniqueLoader(yaml.SafeLoader):
    def construct_mapping(self, node, deep=False):
        keys = set()
        for key, _ in node.value:
            value = self.construct_object(key, deep=False)
            if value in keys:
                raise ConfigError(f'duplicate YAML key: {value!r}')
            keys.add(value)
        return super().construct_mapping(node, deep=deep)


class ExpandedDumper(yaml.SafeDumper):
    def ignore_aliases(self, data):
        return True


def load(path):
    return yaml.load(path.read_text(encoding='utf-8'), Loader=UniqueLoader)


def repo_path(value):
    result = (ROOT / value).resolve()
    if not result.is_relative_to(ROOT):
        raise ConfigError(f'resource escapes repository: {value}')
    return result


def resource_file(resource, environment=None):
    if 'repo_path' in resource:
        result = repo_path(resource['repo_path'])
        label = resource['repo_path']
    else:
        fallback = repo_path(resource['default_repo_path'])
        environment = os.environ if environment is None else environment
        override = environment.get(resource['environment'], '').strip()
        # Environment overrides can name deployment-installed files outside
        # the repository. Relative values resolve from ROOT, like repo paths.
        result = (ROOT / override).resolve() if override else fallback
        label = f"environment resource {resource['environment']}"
    require(result.is_file(), f'missing {label}')
    return result


def digest(path):
    return hashlib.sha256(path.read_bytes().replace(b'\r\n', b'\n')).hexdigest()


def require(condition, message):
    if not condition:
        raise ConfigError(message)


def unique(items, field, label):
    values = [item[field] for item in items]
    require(len(values) == len(set(values)), f'duplicate {label} {field}')


def walk(value):
    yield value
    if isinstance(value, dict):
        for child in value.values():
            yield from walk(child)
    elif isinstance(value, list):
        for child in value:
            yield from walk(child)


def check_schema(document):
    require(isinstance(document, dict), 'configuration document must be an object')
    errors = sorted(VALIDATOR.iter_errors(document), key=lambda e: list(map(str, e.path)))
    if errors:
        error = errors[0]
        raise ConfigError(f"schema at {'/'.join(map(str, error.path)) or '/'}: {error.message}")


def validate_workflow(document):
    workflow = document['workflow']
    unique(workflow['bays'], 'id', 'bay')
    bays = {b['id'] for b in workflow['bays']}
    settings = workflow['settings']
    if workflow['handler'] == 'ekch/v1':
        require(document['airport'] == 'EKCH', 'EKCH adapter cannot be assigned to another airport')
        require(document['ownership']['strips'] == 'route' and document['ownership']['sectors'] == 'coverage', 'EKCH adapter requires existing ownership')
        source = (ROOT / 'backend/internal/shared/bays.go').read_text(encoding='utf-8')
        required = set(re.findall(r'BAY_[A-Z_]+\s*=\s*"([A-Z_]+)"', source))
        require(required <= bays, 'missing legacy EKCH bay')
        require(document['geography']['latitude'] == 55.6181 and document['geography']['longitude'] == 12.656 and document['geography']['elevation_ft'] == 17 and settings['relevant_distance_nm'] == 30, 'EKCH helper still requires its legacy geography/distance')
    else:
        require(document['ownership']['strips'] == 'none' and document['ownership']['sectors'] == 'none', 'paper example requires ownership disabled')
        require(settings['initial_bay'] in bays and settings['runway_bay'] in bays, 'unknown paper initial/runway bay')
        require(settings['initial_bay'] != settings['runway_bay'], 'paper initial and runway bays must differ')
        require(set(settings['bay_ground_states']) <= bays, 'unknown bay-to-ES key')
        require(set(settings['ground_state_bays'].values()) <= bays, 'unknown ES-to-bay target')


def validate_airport(document, environment=None):
    check_schema(document)
    unique(document['positions'], 'name', 'position')
    unique(document['coverage']['responsibilities'], 'id', 'responsibility rule')
    unique(document['routing']['ground'], 'id', 'ground route')
    unique(document['routing']['airborne'], 'name', 'airborne route')
    positions = {p['name'] for p in document['positions']}
    runways = set(document['geography']['runways'])
    duties = document['coverage']['responsibilities']
    roles = {d['name'] for d in duties} | {d.get('key', d['name']) for d in duties}
    def position(value): require(value in positions, f'unknown position: {value}')
    def runway(value): require(str(value).zfill(2) in runways, f'unknown runway: {value}')
    for key in document['geography']['initial_cfl_ft']:
        runway(key)
    for node in walk(document):
        if isinstance(node, dict) and 'repo_path' in node:
            require(repo_path(node['repo_path']).is_file(), f"missing repository resource: {node['repo_path']}")
    region_names = set()
    if 'regions' in document['geography']:
        regions = json.loads(resource_file(document['geography']['regions'], environment).read_text(encoding='utf-8'))
        region_names = {f['properties'].get('name', f['properties'].get('Name')) for f in regions['features']}
    for duty in duties:
        for provider in duty['providers']:
            position(provider)
        for active in duty.get('active', []):
            runway(active)
        for region in duty.get('region', []):
            require(region in region_names, f'unknown region: {region}')
    for provider in document['coverage']['airborne_fallback']:
        position(provider)
    for priority in document['master']['priorities']:
        position(priority['position'])
    unique(document['master']['priorities'], 'position', 'master priority')
    routes = document['routing']['ground']
    for route in routes:
        if 'forRunway' in route:
            runway(route['forRunway'])
        for active in route.get('active', []):
            runway(active)
        for role in route['path']:
            require(role in roles, f'unknown route responsibility: {role}')
        for original, target in route.get('owner_overrides', {}).items():
            require(original in roles and target in roles, 'unknown route owner override')
        for selector in route.get('forStandRanges', []):
            if isinstance(selector, dict):
                require(selector['from'] <= selector['to'], 'stand range is inverted')
    require(not document['routing']['airborne'] or sum(r['default'] for r in document['routing']['airborne']) == 1, 'airborne routes require exactly one default')
    for runway_name, provider in document['routing']['missed_approach'].items():
        runway(runway_name)
        position(provider)
    view = document['view']
    for provider, variants in view['layouts'].items():
        position(provider)
        for variant in variants:
            for active in variant.get('active', []):
                runway(active)
            for matcher in variant['online'] + variant['offline']:
                require(matcher in positions or matcher in {'_GND', '_TWR', '_APP', '_DEL', '_DEP', '_CTR'}, f'unknown layout matcher: {matcher}')
    for providers in view['message_areas'].values():
        for provider in providers:
            position(provider)
    validate_workflow(document)
    for service in document['services'].values():
        if service.get('availability') == 'disabled':
            continue
        for key in ['rateUri', 'sidIntervalUri', 'taxizonesUri']:
            if key in service.get('config', {}):
                uri = service['config'][key]
                if not uri.startswith(('https://', 'http://')):
                    require(repo_path(str(Path(service['config_root']) / uri)).is_file(), f'missing CDM data: {uri}')
    return document


def write_yaml(path, document):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text('# Worked proposal example; not loaded by the current application.\n' + yaml.dump(document, Dumper=ExpandedDumper, sort_keys=False, allow_unicode=True, width=100), encoding='utf-8')


def migrated_ekch():
    legacy_path = ROOT / 'backend/config/ekch.yaml'
    old = load(legacy_path)
    bays_source = (ROOT / 'backend/internal/shared/bays.go').read_text(encoding='utf-8')
    hub_source = (ROOT / 'backend/internal/euroscope/hub.go').read_text(encoding='utf-8')
    bay_ids = list(dict.fromkeys(re.findall(r'BAY_[A-Z_]+\s*=\s*"([A-Z_]+)"', bays_source)))
    duties = []
    for i, original in enumerate(old['sectors'], 1):
        duty = {'id': f'responsibility-{i:03}', **copy.deepcopy(original)}
        duty['providers'] = duty.pop('owner')
        duties.append(duty)
    routes = [{'id': f'ground-route-{i:03}', **copy.deepcopy(route)} for i, route in enumerate(old['routes'], 1)]
    master_source = hub_source.split('func masterClientPriority(')[1].split('\nfunc preferredMasterClient')[0]
    first = re.search(r'callsign == "([A-Z0-9_]+)"', master_source).group(1)
    master_names = [first, *re.findall(r'case "([A-Z0-9_]+)":', master_source)]
    resource = lambda name: {'repo_path': name}
    elevation = int(re.search(r'AirportElevation\s*=\s*(\d+)', bays_source).group(1))
    distance = int(re.search(r'RelevantDistance\s*=\s*(\d+)', bays_source).group(1))
    navigation_source = 'backend/internal/navigation/config.go'
    terminal_path = re.search(r'DefaultTerminalGeometryPath\s*=\s*"([^"]+)"', (ROOT / navigation_source).read_text(encoding='utf-8')).group(1)
    source_paths = ['backend/config/ekch.yaml', 'backend/internal/shared/bays.go', 'backend/internal/euroscope/hub.go', 'backend/config/ekch_regions.json', navigation_source]
    airport = {
        '$schema': '../schema/airport-configuration.schema.json', 'schema_version': 'flightstrips-airport/v1', 'kind': 'airport', 'airport': 'EKCH',
        'geography': {'latitude': old['latitude'], 'longitude': old['longitude'], 'elevation_ft': elevation, 'transition_altitude_ft': old['transition_altitude'], 'runways': old['runways'], 'initial_cfl_ft': old['runway_initial_cfl'], 'regions': resource('backend/config/ekch_regions.json')},
        'positions': old['positions'], 'coverage': {'matching': {'primary_frequency': True, 'tuned_frequencies': True, 'callsign_fallback': True, 'normalization': 'vatsim/v1', 'provider_selection': 'existing-coverage/v1'}, 'responsibilities': duties, 'airborne_fallback': old['airborne_owners'], 'membership': {'policy': 'legacy-single-airport/v1'}},
        'master': {'priorities': [{'position': p, 'match': 'primary-frequency-or-callsign' if i == 0 else 'resolved-primary-frequency-then-callsign'} for i, p in enumerate(master_names)], 'preempt_higher_priority': True, 'equal_priority': 'retain-current', 'fallback': 'any-non-observer-participant', 'observers_eligible': False},
        'ownership': {'strips': 'route', 'sectors': 'coverage', 'callsign_prefixes': old['owner_callsign_prefixes']},
        'routing': {'ground': routes, 'airborne': old['airborne_routes'], 'missed_approach': old['missed_approach_handover']},
        'view': {'id': 'ekch', 'layouts': old['layouts'], 'airborne_fallback_layout': old['airborne_fallback_layout'], 'message_areas': old['message_areas']},
        'workflow': {'handler': 'ekch/v1', 'settings': {'airborne_altitude_agl_ft': old['airborne_altitude_agl'], 'relevant_distance_nm': distance}, 'bays': [{'id': b, 'label': b} for b in bay_ids]},
        'services': {
            'cdm': {'availability': 'deployment', 'config_root': 'backend/config', 'config': old['cdm']},
            'stand_assignment': {'availability': 'deployment', 'stands': resource('backend/config/ekch/GRpluginStands.txt'), 'aircraft_reference': resource('backend/config/ekch/GRpluginAircraftInfo.txt'), 'aircraft_icao': {'environment': 'GRPLUGIN_ICAO_AIRCRAFT_JSON', 'default_repo_path': 'backend/config/data/ICAO_Aircraft.json'}, 'airline_assignment': resource('backend/config/ekch/airline_assignment.json'), 'scenery': resource('backend/config/ekch/gsx_sceneries.json')},
            'text_pilot': {'availability': 'deployment', 'provider': 'frequency-message/v1', 'layout_data': resource('frontend/src/components/text-pilot/storyboard-layouts.json')},
            'aman': {'availability': 'deployment', 'terminal': resource('backend/' + terminal_path), 'boundary': resource('backend/config/aman/boundaries/EKCH.json')},
            'pdc': {'availability': 'deployment', 'validation': old['pdc_validation']}, 'ecfmp': {'availability': 'deployment'},
        },
        'validation': {'clearance': old['clx_validation'], 'taxiway_types': old['taxiway_type_validation']},
        'provenance': [{'path': p, 'sha256': digest(ROOT / p)} for p in source_paths],
    }
    return airport


def legacy_from_airport(new):
    sectors = []
    for duty in new['coverage']['responsibilities']:
        sector = {k: copy.deepcopy(v) for k, v in duty.items() if k not in {'id', 'providers'}}
        sector['owner'] = copy.deepcopy(duty['providers'])
        sectors.append(sector)
    return {
        'latitude': new['geography']['latitude'], 'longitude': new['geography']['longitude'],
        'transition_altitude': new['geography']['transition_altitude_ft'], 'runway_initial_cfl': new['geography']['initial_cfl_ft'],
        'cdm': new['services']['cdm']['config'], 'clx_validation': new['validation']['clearance'],
        'positions': new['positions'], 'owner_callsign_prefixes': new['ownership']['callsign_prefixes'], 'airborne_owners': new['coverage']['airborne_fallback'],
        'airborne_fallback_layout': new['view']['airborne_fallback_layout'], 'airborne_altitude_agl': new['workflow']['settings']['airborne_altitude_agl_ft'],
        'airborne_routes': new['routing']['airborne'], 'sectors': sectors,
        'routes': [{k: v for k, v in route.items() if k != 'id'} for route in new['routing']['ground']],
        'message_areas': new['view']['message_areas'], 'runways': new['geography']['runways'], 'layouts': new['view']['layouts'],
        'missed_approach_handover': new['routing']['missed_approach'], 'pdc_validation': new['services']['pdc']['validation'], 'taxiway_type_validation': new['validation']['taxiway_types'],
    }


def main():
    ekch_path = DESIGN / 'examples/ekch.airport.yaml'
    fake_path = DESIGN / 'examples/zzzz.airport.yaml'
    if ARGS.refresh_ekch:
        write_yaml(ekch_path, migrated_ekch())
    files = [Path(f).resolve() for f in ARGS.files] if ARGS.files else [ekch_path, fake_path]
    validated = []
    for file in files:
        validate_airport(load(file))
        validated.append(file.relative_to(ROOT).as_posix())
    airport, fake = load(ekch_path), load(fake_path)
    old = load(ROOT / 'backend/config/ekch.yaml')
    require(legacy_from_airport(airport) == old, 'EKCH roundtrip lost, changed, or omitted current YAML settings')
    require(airport == migrated_ekch(), 'EKCH example differs from current sources and code-derived constants')
    negatives = []
    def rejects(label, mutation, base=airport, environment=None):
        candidate = copy.deepcopy(base)
        mutation(candidate)
        try:
            validate_airport(candidate, environment)
        except ConfigError:
            negatives.append(label)
            return
        raise ConfigError(f'invalid configuration accepted: {label}')
    rejects('null placeholder', lambda c: c.update(coverage=None))
    rejects('unknown root setting', lambda c: c.update(current_airport='EKCH'))
    rejects('unknown coverage provider', lambda c: c['coverage']['responsibilities'][0]['providers'].append('ZZZZ_APP'))
    rejects('unknown route responsibility', lambda c: c['routing']['ground'][0]['path'].append('UNKNOWN_ROLE'))
    rejects('unknown runway', lambda c: c['routing']['ground'][0].update(forRunway='99'))
    rejects('frequency must stay a string', lambda c: c['positions'][0].update(frequency=119.905))
    rejects('duplicate position', lambda c: c['positions'].append(copy.deepcopy(c['positions'][0])))
    rejects('duplicate responsibility ID', lambda c: c['coverage']['responsibilities'].append(copy.deepcopy(c['coverage']['responsibilities'][0])))
    rejects('session-owned feed cache', lambda c: c['services'].update(vatsim={'availability': 'enabled'}))
    rejects('unknown service setting', lambda c: c['services']['cdm'].update(poller_per_session=True))
    rejects('airport-local PDC credentials', lambda c: c['services']['pdc'].update(logon='example-only'))
    rejects('session-owned PDC poller', lambda c: c['services']['pdc'].update(poller_per_session=True))
    rejects('observer master', lambda c: c['master'].update(observers_eligible=True))
    rejects('missing referenced file', lambda c: c['geography']['regions'].update(repo_path='backend/config/missing_regions.json'))
    rejects('repository traversal', lambda c: c['geography']['regions'].update(repo_path='../outside.json'))
    region_environment = 'FLIGHTSTRIPS_EXAMPLE_REGIONS'
    region_reference = {'environment': region_environment, 'default_repo_path': 'backend/config/ekch_regions.json'}
    with_environment = copy.deepcopy(airport)
    with_environment['geography']['regions'] = region_reference
    validate_airport(with_environment, {})
    validate_airport(with_environment, {region_environment: '  '})
    with_override = copy.deepcopy(with_environment)
    with_override['geography']['regions']['default_repo_path'] = 'backend/config/missing_regions.json'
    validate_airport(with_override, {region_environment: str(ROOT / 'backend/config/ekch_regions.json')})
    rejects('missing selected region environment file', lambda c: None, with_environment,
            {region_environment: str(ROOT / 'backend/config/missing_regions.json')})
    rejects('missing region fallback file', lambda c: None, with_override, {})
    rejects('unknown handler', lambda c: c['workflow'].update(handler='dynamic-plugin'))
    rejects('unknown handler setting', lambda c: c['workflow']['settings'].update(invented=True))
    rejects('cross-handler settings', lambda c: c['workflow']['settings'].update(initial_bay='HOLD'))
    rejects('zero airborne threshold', lambda c: c['workflow']['settings'].update(airborne_altitude_agl_ft=0))
    rejects('EKCH geography constants not extracted yet', lambda c: c['geography'].update(latitude=1))
    rejects('EKCH handler on another airport', lambda c: c.update(airport='ZZZZ'))
    rejects('missing EKCH bay', lambda c: c['workflow']['bays'].pop())
    rejects('separate profile reference no longer accepted', lambda c: c['workflow'].update(profile={'repo_path':'old.yaml'}))
    rejects('script handler not accepted', lambda c: c['workflow'].update(script='run.js'))
    rejects('unknown paper initial bay', lambda c: c['workflow']['settings'].update(initial_bay='UNKNOWN'), fake)
    rejects('same paper initial/runway bay', lambda c: c['workflow']['settings'].update(initial_bay='RUNWAY'), fake)
    rejects('unknown paper ES target', lambda c: c['workflow']['settings']['ground_state_bays'].update(TAXI='UNKNOWN'), fake)
    rejects('unknown paper movement state', lambda c: c['workflow']['settings']['bay_ground_states'].update(HOLD='FLY'), fake)
    rejects('wrong annotation default type', lambda c: c['workflow']['annotations']['review_required'].update(default='false'), fake)
    rejects('missing custom annotation', lambda c: c['workflow']['annotations'].clear(), fake)
    rejects('duplicate bay', lambda c: c['workflow']['bays'].append(copy.deepcopy(c['workflow']['bays'][0])), fake)
    rejects('paper ownership enabled', lambda c: c['ownership'].update(strips='route'), fake)
    disabled = copy.deepcopy(airport)
    disabled['services']['cdm'] = {'availability':'disabled'}
    validate_airport(disabled)
    report = {'schema':'flightstrips-airport/v1', 'validation':'JSON Schema 2020-12 plus semantic/reference checks', 'validated_documents':validated,
              'ekch_roundtrip':{'source':'backend/config/ekch.yaml', 'source_sha256':digest(ROOT / 'backend/config/ekch.yaml'), 'all_top_level_settings_preserved':sorted(old), 'positions':len(old['positions']), 'sector_variants':len(old['sectors']), 'ground_routes':len(old['routes']), 'airborne_routes':len(old['airborne_routes']), 'layout_positions':len(old['layouts']), 'master_priority':[r['position'] for r in airport['master']['priorities']], 'bay_ids':[b['id'] for b in airport['workflow']['bays']]},
              'rejected_invalid_cases':negatives,
              'resource_resolution_checks':['Unset region environment uses the repository fallback.', 'Blank region environment uses the repository fallback.', 'An explicit region environment file takes precedence over a missing fallback.'],
              'limits':['No production startup, persistence, transport, or complete EKCH handler migration is wired.', 'Executable handler proof covers bay movement and full-flight observation decisions only.', 'Current YAML data preservation does not establish complete runtime behavior equivalence.', 'ZZZZ and all its operating values are fictional and must not be deployed.']}
    if ARGS.write_report:
        (HERE / 'validation-report.json').write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8')
    print(f'Validated {len(validated)} combined airport documents; preserved all {len(old)} EKCH settings; rejected {len(negatives)} invalid cases.')
    return 0


if __name__ == '__main__':
    try:
        raise SystemExit(main())
    except (ConfigError, yaml.YAMLError, OSError) as error:
        raise SystemExit(f'Configuration validation failed: {error}') from error
