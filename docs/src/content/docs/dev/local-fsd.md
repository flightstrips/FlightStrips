---
title: Local FSD Server
description: How to setup a local FSD server for testing
---



### Download license file.
The Euorsocpe FSD server require a license file, wich can be downloaded from their website.
https://www.euroscope.hu/license/PublicVatsimLicence.txt 

This file needs to be saved with a `.lic` extension instead of `.txt` 

:::danger[Modify the license file]
Before loading it. Remove the `LICENCE:` from the file, and remove any trailing white space.
:::

Now start the `Euroscope FSD Server` it will ask for the license file on first start
Once started, go to senarios and load our testing senario from [here](https://hellow.word)

Your FSD Server is now running on localhost and ready for testing.
:::note
Only two active connections are supported for this FSD Server
:::