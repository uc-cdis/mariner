### Browsing and Retrieving Output From A Workflow Run

Mariner implicitly depends on the existence of a user data client,
an API where users can browse/upload/download/delete files
from their user data space, which is persistent storage
on the Gen3/commons side for data which belongs to a user
and is not commons data.

The user data space is where a user can stage files to be input
to a workflow run, and theoretically, also the same place
where users can stage input files for any "app on Gen3", e.g., a Jupyter notebook.

The user data space (also could be called an "analysis space") is also
where output files from apps are stored.

Currently, there's an S3 bucket which is a dedicated user data space,
where keys at the root are userID's, and any file which belongs to user_A
has `user_A/` as a prefix. Per workflow run, there is a working directory
created and dedicated to that run, under that user's prefix in that S3 bucket.
All files generated by the workflow run are written to this working directory,
and any files which are not explicitly listed as output files of the top-level workflow
(i.e., all intermediate files) get deleted at the end of the run so that only
the desired output files are kept.

Currently there does not exist a Gen3 user-data-client,
so in order to browse and retrieve your output files from
the workflow's working directory in S3,
you must use the [AWS S3 CLI](https://docs.aws.amazon.com/cli/latest/reference/s3/) directly.
