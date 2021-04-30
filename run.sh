#!/busybox/sh -x

results_dir="${RESULTS_DIR:-/tmp/results}"

mkdir -p ${results_dir}
cd ${results_dir}
conformance -test.v >report.log
cat report.log
cat report.log | go-junit-report >report.xml
tar czf results.tar.gz *
echo ${results_dir}/results.tar.gz >${results_dir}/done
