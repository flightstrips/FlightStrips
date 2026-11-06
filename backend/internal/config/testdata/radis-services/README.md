# RADIS service acceptance fixtures

These definitions and resources are synthetic test inputs, not approved EKBI/EKYT operating data. They must never be selected by deployment configuration. Geography, runway 09/27, frequencies 118.000/118.005/119.000, S1/S2/S3 stands, bay mappings, rates, taxi/deice timings, removal and recovery settings are invented and deliberately differ between service scopes. PDC/AMAN remain disabled.

The app assembly test and `browserharness -suite radis` explicitly load these files with the production typed loader. Aircraft references reuse the existing shared reference dataset; policy, stands, text and CDM inputs remain airport-bound. Empty CDM collections are deliberate test inputs with explicit synthetic defaults, not live procedure/rate approval. The minimal fixtures in the parent directory continue testing service-disabled deployments.

MA-08 activation must replace these with reviewed geography, coverage, native field/bay mapping, runway/ATIS authority, stand/procedure/CDM/text data, threshold/timestamp/recovery policies and controller operational acceptance. No production airport selection is added here.
